package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"net/smtp"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func main() {
	sendEmail := flag.Bool("e", false, "email query result")
	maxWorker := flag.Int("w", 1, "max network fetch worker")
	flag.Parse()
	params := flag.Args()
	if len(params) != 3 {
		fmt.Println("Usage: tenant [douban group id] [max page] [search regexp]")
		return
	}
	groupID := params[0]
	searchKey := params[2]
	maxPage, err := strconv.ParseInt(params[1], 10, 64)
	if err != nil {
		fmt.Printf("[ERROR] parse 'max page': %v\n", err)
		return
	}
	dcs, err := search(groupID, int(maxPage), *maxWorker, searchKey)
	fmt.Printf("[QUERY] 在豆瓣小组'%s'中用关键字'%s'搜索了%d页后得到%d个匹配的帖子：\n",
		groupID, searchKey, maxPage, len(dcs))
	if len(dcs) > 0 {
		fmt.Println("        ********************************************************")
		for _, d := range dcs {
			fmt.Printf("        * %v\n", d)
		}
		fmt.Println("        ********************************************************")
	}
	if err != nil {
		fmt.Printf("[QUERY] errors occurred during concurrent search: %v\n", err)
	}
	if len(dcs) > 0 && *sendEmail {
		renderer, err := loadTempl("search#result")
		if err != nil {
			fmt.Printf("[ERROR] load template': %v\n", err)
			return
		}
		var report bytes.Buffer
		if err := renderer.Execute(&report, struct {
			Group     string
			Max       int
			Key       string
			Created   time.Time
			Discusses []*discuss
		}{groupID, int(maxPage), searchKey, time.Now(), dcs}); err != nil {
			fmt.Printf("[ERROR] render template: %v\n", err)
			return
		}
		if err := sendMail("Douban Search Report", report.String()); err != nil {
			fmt.Printf("[ERROR] send email: %v\n", err)
			return
		}
		fmt.Println("[EMAIL] 搜索报告发送成功")
	}
}

func search(group string, pglimit int, mxworker int, srkey string) ([]*discuss, error) {
	key, err := regexp.Compile(srkey)
	if err != nil {
		return nil, fmt.Errorf("compile search key '%s' as regexp: %v", srkey, err)
	}
	filter := func(d *discuss) bool {
		return key.Match([]byte(d.Title))
	}
	type outcome struct {
		Out   []*discuss
		Wrong error
	}
	ocqueue := make(chan outcome)
	worker := func(lowlimit int, toplimit int) {
		var wrong error
		out := make([]*discuss, 0)
		for n := lowlimit; n <= toplimit; n++ {
			url := fmt.Sprintf("https://www.douban.com/group/%s/discussion?start=%d",
				group, n*25+1)
			if dcs, err := filterDiscuss(url, filter); err != nil {
				wrong = fmt.Errorf("__P%d__%v;", n, err)
				break
			} else {
				out = append(out, dcs...)
			}
		}
		ocqueue <- outcome{Out: out, Wrong: wrong}
	}
	step := pglimit/mxworker + 1
	for n := 0; n < mxworker; n++ {
		lowlimit := n * step
		toplimit := lowlimit + step - 1
		if toplimit > pglimit {
			toplimit = pglimit
		}
		go worker(lowlimit, toplimit)
	}
	dcs := make([]*discuss, 0)
	ers := new(bytes.Buffer)
	for n := 0; n < mxworker; n++ {
		outcome := <-ocqueue
		dcs = append(dcs, outcome.Out...)
		if outcome.Wrong != nil {
			ers.WriteString(outcome.Wrong.Error())
		}
	}
	if ers.Len() == 0 {
		return dcs, nil
	}
	return dcs, fmt.Errorf("interrupt with errors: %v", ers.String())
}

type discuss struct {
	Title     string
	Link      string
	ID        string
	Author    string
	Reply     int
	LastReply time.Time
}

func (d *discuss) String() string {
	const widthLimit = 80
	if len(d.Title) > widthLimit {
		index := widthLimit
		for !utf8.RuneStart(d.Title[index]) {
			index--
		}
		return d.Title[:index] + "..."
	}
	return d.Title
}

func filterDiscuss(url string, filter func(*discuss) bool) ([]*discuss, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("abnormal response status code: %s", resp.Status)
	}
	root, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}
	if titleNode, ok := scrape.Find(root, scrape.ByTag(atom.Title)); ok &&
		scrape.Text(titleNode) == "豆瓣" {
		return nil, fmt.Errorf("incorrect html title '豆瓣'")
	}

	matches := scrape.FindAll(root, func(n *html.Node) bool {
		if n.DataAtom == atom.A && n.Parent != nil && n.Parent.DataAtom == atom.Td &&
			scrape.Attr(n.Parent, "class") == "title" {
			return true
		}
		return false
	})
	if len(matches) == 0 {
		return nil, fmt.Errorf("blank page matches no discuss link")
	}

	var dcs = make([]*discuss, 0, 25)
	for _, linkNode := range matches {
		var d discuss
		d.Link = scrape.Attr(linkNode, "href")
		link := strings.TrimRight(d.Link, "/")
		d.ID = link[strings.LastIndex(link, "/")+1:]
		d.Title = scrape.Attr(linkNode, "title")
		authorNode := linkNode.Parent.NextSibling.NextSibling
		d.Author = scrape.Text(authorNode)
		replyNode := authorNode.NextSibling.NextSibling
		replyStr := scrape.Text(replyNode)
		if replyStr == "" {
			replyStr = "0"
		}
		reply, err := strconv.Atoi(replyStr)
		if err != nil {
			return nil, fmt.Errorf("convert reply-num for '%s': %v", d.Title, err)
		}
		d.Reply = reply
		lastReplyNode := replyNode.NextSibling.NextSibling
		lastReplyStr := scrape.Text(lastReplyNode)
		if len(lastReplyStr) == 10 {
			lastReplyStr = fmt.Sprintf("%s 00:00 CST", lastReplyStr)
		} else {
			lastReplyStr = fmt.Sprintf("%d-%s CST", time.Now().Year(), lastReplyStr)
		}
		lastReply, err := time.Parse("2006-01-02 15:04 MST", lastReplyStr)
		if err != nil {
			return nil, fmt.Errorf("convert last-reply-time for '%s': %v", d.Title, err)
		}
		d.LastReply = lastReply
		if filter == nil || filter(&d) {
			dcs = append(dcs, &d)
		}
	}
	return dcs, nil
}

func sendMail(subject, body string) error {
	from := "jaysinco@qq.com"
	to := "jaysinco@163.com"
	pwd := "ygkstvxfsovkific"
	domain := from[strings.Index(from, "@")+1:]
	auth := smtp.PlainAuth("", from, pwd, fmt.Sprintf("smtp.%s", domain))
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"Subject: %s\r\n"+
		"\r\n%s\r\n", from, to, subject, body)
	return smtp.SendMail(fmt.Sprintf("smtp.%s:25", domain), auth,
		from, strings.Split(to, ";"), []byte(msg))
}

func loadTempl(name string) (*template.Template, error) {
	tmpl := `
<style type='text/css'> 
	a:link { text-decoration: none; color: #37a; background: transparent; } 
	a:visited { text-decoration: none; color: #666699; background : transparent; }
	a:hover { color: #FFFFFF; text-decoration: none; background: #37a; }
	div.basic{ width:100%; background:#fff4e8; font-size:13px; word-wrap:break-word; word-break:break-all; }
	table.gridtable { width:100%; font-size:13px; color:#333333; border-width: 1px; border-color: #666666; border-collapse: collapse; }
	table.gridtable th { border-width: 1px; padding: 8px; border-style: solid; border-color: #666666; background-color: #dedede; }
	table.gridtable td { border-width: 1px; padding: 8px; border-style: solid; border-color: #666666; background-color: #ffffff; }
</style> 
<h1>豆瓣小组搜索报告</h1>
<div class='basic'>
	<br><b>小组代码: &nbsp;</b>{{.Group}} 
	<br><b>搜索页数: &nbsp;</b>{{.Max}}
	<br><b>筛选数量: &nbsp;</b>{{.Discusses | len}}
	<br><b>生成时间: &nbsp;</b>{{.Created.Format "2006-01-02 15:04:05"}}
	<br><b>关键词: &nbsp;</b>
	<br><b>==>&nbsp;</b>{{.Key}}
	<br><br>
</div>
<table class='gridtable'>
	<tr>
		<th width='70%'><b>话题</b></th>	
		<th><b>回应</b></th>
	</tr>
	{{range .Discusses}}
	<tr>
		<td><a href='{{.Link}}'>{{.Title}}</a></td>
		<td>{{.Reply}}</td>	
	</tr>
	{{end}}
</table>`
	renderer, err := template.New(name).Parse(string(tmpl))
	if err != nil {
		return nil, fmt.Errorf("parse template: %v", err)
	}
	return renderer, nil
}
