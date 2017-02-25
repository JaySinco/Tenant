package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"net/smtp"
	"os"
	"regexp"
	"strings"
	"time"
)

var groupFlag = flag.String("group", "shanghaizufang", "group name")
var maxFlag = flag.Int("n", 25, "max num of posts")
var keyFlag = flag.String("key",
	"真北路|大渡河|金沙江路|娄山关路|威宁路|北新泾|淞虹路|中山公园|延安西路|虹桥路|"+
		"曹杨路|上海体育馆|桂林路|漕河泾开发区|合川路|伊犁路|宋园路|水城路|龙溪路|"+
		"宜山路|上海动物园|龙柏新村|紫藤路|虹桥1号航站楼",
	"search key pattern")

var fromFlag = flag.String("from", "zxk156@qq.com", "email sender")
var toFlag = flag.String("to", "jaysinco@163.com", "email receivers(';' separated)")
var pwdFlag = flag.String("p", "", "email password")

var keyRegxp *regexp.Regexp
var postsPicked = make([]*Post, 0, 10)
var dupCheck = make(map[string]bool)

func main() {
	flag.Parse()
	fmt.Fprintln(os.Stderr, "filtering posts...")
	keyRegxp = regexp.MustCompile(*keyFlag)
	if err := (&Group{*groupFlag, *maxFlag}).EachPost(filter); err != nil {
		fmt.Fprintf(os.Stderr, "main: filter post: %v\n", err)
	}
	fmt.Fprintln(os.Stderr, "fetching detail...")
	done := make(chan error)
	limit := make(chan struct{}, 10)
	suceed := 0
	for _, pt := range postsPicked {
		post := pt
		go func() {
			limit <- struct{}{}
			done <- post.GetDetail()
			<-limit
		}()
	}
	for i := 0; i < len(postsPicked); i++ {
		if err := <-done; err == nil {
			suceed += 1
		}
	}
	fmt.Fprintln(os.Stderr, "generating report...")
	var buf bytes.Buffer
	if err := template.Must(template.New("Post Report").
		Parse(postReport)).Execute(&buf, Report{
		Group:   *groupFlag,
		Key:     *keyFlag,
		Max:     *maxFlag,
		Suceed:  suceed,
		Created: time.Now(),
		Posts:   postsPicked,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "main: render report: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "sending report...")
	if *pwdFlag == "" {
		fmt.Fprintln(os.Stdout, buf.String())
	} else {
		date := time.Now().Format("2006-01-02")
		reportMail := SMTPMail{
			From:     *fromFlag,
			To:       *toFlag,
			Password: *pwdFlag,
			Subject:  fmt.Sprintf("%s %s", date, "豆瓣小组报告"),
			Body:     buf.String(),
		}
		if err := reportMail.Send(); err != nil {
			fmt.Fprintf(os.Stderr, "main: send email: %v\n", err)
		}
	}
	fmt.Fprintln(os.Stderr, "program completed!")
}

func filter(p *Post) error {
	if keyRegxp.Match([]byte(p.Title)) && !dupCheck[p.ID] {
		postsPicked = append(postsPicked, p)
		dupCheck[p.ID] = true
	}
	return nil
}

type SMTPMail struct {
	From     string
	To       string
	Password string
	Subject  string
	Body     string
}

func (e *SMTPMail) Send() error {
	domain := e.From[strings.Index(e.From, "@")+1:]
	auth := smtp.PlainAuth("", e.From, e.Password,
		fmt.Sprintf("smtp.%s", domain))
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"Subject: %s\r\n"+
		"\r\n%s\r\n", e.From, e.To, e.Subject, e.Body)
	if err := smtp.SendMail(fmt.Sprintf("smtp.%s:25", domain), auth,
		e.From, strings.Split(e.To, ";"), []byte(msg)); err != nil {
		return err
	}
	return nil
}

type Report struct {
	Group   string
	Max     int
	Suceed  int
	Key     string
	Created time.Time
	Posts   []*Post
}

const postReport = `
<style type="text/css"> 
	a:link {
		text-decoration: none;
		color: #37a;
		background: transparent;
	} 
	a:visited {
		text-decoration: none;
		color: #666699;
		background : transparent;
	}
	a:hover {
	    color: #FFFFFF;
	    text-decoration: none;
	    background: #37a;
	}
	div.basic{ 
		width:100%; 
		background:#fff4e8; 
		font-size:13px;
		word-wrap:break-word;    
		word-break:break-all;
	}
	table.gridtable {
		width:100%;
		font-size:13px;
		color:#333333;
		border-width: 1px;
		border-color: #666666;
		border-collapse: collapse;
	}
	table.gridtable th {
		border-width: 1px;
		padding: 8px;
		border-style: solid;
		border-color: #666666;
		background-color: #dedede;
	}
	table.gridtable td {
		border-width: 1px;
		padding: 8px;
		border-style: solid;
		border-color: #666666;
		background-color: #ffffff;
	}
</style> 
<h1>
	豆瓣小组报告
</h1>
<div class="basic">
	<br>
	<b>小组代码: &nbsp;</b>{{.Group}} 
	<br>
	<b>搜索数量: &nbsp;</b>{{.Max}}
	<br>
	<b>筛选数量: &nbsp;</b>{{.Posts | len}}
	<br>
	<b>详细数量: &nbsp;</b>{{.Suceed}}
	<br>
	<b>生成时间: &nbsp;</b>{{.Created.Format "2006-01-02 15:04:05"}}
	<br>
	<b>关键词: &nbsp;</b>
	<br>
	  <b>==>&nbsp;</b>{{.Key}}
	<br>
	<br>
</div>
<br>
<table class="gridtable">
	<tr>
		<th><b>创建</b></th>	
		<th><b>回应</b></th>
 		<th width="70%"><b>话题</b></th>		
 		<th><b>喜欢</b></th>
 	</tr>
	{{range .Posts}}
	<tr>
		<td>{{.Created.Format "2006-01-02"}}</a></td>
		<td>{{.Reply}}</td>
		<td><a href="{{.Link}}">{{.Title}}</a></td>
		<td>{{.Favor}}</td>
	</tr>
	{{end}}
</table>`
