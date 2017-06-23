package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func main() {
	// load config
	cfg, err := loadConfig("config.json")
	if err != nil {
		log.Printf("[ERROR] load configure from 'config.json': %v", err)
		return
	}

	// search discuss following config
	log.Printf("[INFO ] search '%s' from group '%s' in %d pages using %d workers",
		cfg.SearchKey, cfg.GroupID, cfg.MaxPage, cfg.MaxWorker)
	dcs, err := search(cfg.GroupID, cfg.MaxPage, cfg.MaxWorker, cfg.SearchKey)
	if err != nil {
		log.Printf("[ERROR] errors occurred during concurrent search: %v", err)
	}
	log.Printf("[INFO ] %d discusses found", len(dcs))
	if len(dcs) == 0 {
		return // no result then quit, don't report
	}

	// generate search report
	renderer, err := loadTempl(cfg.RenderFile, "report#search")
	if err != nil {
		log.Printf("[ERROR] load template from '%s': %v", cfg.RenderFile, err)
		return
	}
	var report bytes.Buffer
	if err := renderer.Execute(&report, struct {
		Group     string
		Max       int
		Key       string
		Created   time.Time
		Discusses []*Discuss
	}{cfg.GroupID, cfg.MaxPage, cfg.SearchKey, time.Now(), dcs}); err != nil {
		log.Printf("[ERROR] generate report: %v", err)
		return
	}
	log.Printf("[INFO ] search report generated")

	// send search report
	// ----- by email
	if cfg.SMTPMail.Send {
		if err := sendMail(cfg.SMTPMail.From, cfg.SMTPMail.To, cfg.SMTPMail.Token,
			"Report sent by Mr.Robot", report.String()); err != nil {
			log.Printf("[ERROR] send report: %v", err)
			return
		}
		log.Printf("[INFO ] report already sent to '%s' authored by '%s'",
			cfg.SMTPMail.To, cfg.SMTPMail.From)
		return // only send report once either way
	}
	// ----- by file
	filenm := fmt.Sprintf("Rp_%s.html", time.Now().Format("20060102_150405"))
	frp, err := os.Create(filenm)
	if err != nil {
		log.Printf("[ERROR] create report file '%s': %v", filenm, err)
		return
	}
	defer frp.Close()
	if _, err := frp.WriteString(report.String()); err != nil {
		log.Printf("[ERROR] write report into file '%s': %v", filenm, err)
		return
	}
	log.Printf("[INFO ] report already sent to newly created file '%s'", filenm)
}

// search takes group name & max search page number & max search worker & search key
// to filter discusses in douban group concurrently.
func search(group string, pglimit int, mxworker int, srkey string) ([]*Discuss, error) {
	// construct filter function
	key, err := regexp.Compile(srkey)
	if err != nil {
		return nil, fmt.Errorf("compile search key '%s' as regexp: %v", srkey, err)
	}
	filter := func(d *Discuss) bool {
		return key.Match([]byte(d.Title))
	}

	// setup worker goroutine and channel
	type Outcome struct {
		Out   []*Discuss
		Wrong error
	}
	ocqueue := make(chan Outcome)
	worker := func(lowlimit int, toplimit int) {
		var wrong error
		out := make([]*Discuss, 0)
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
		ocqueue <- Outcome{Out: out, Wrong: wrong}
	}

	// create workers
	step := pglimit/mxworker + 1
	for n := 0; n < mxworker; n++ {
		lowlimit := n * step
		toplimit := lowlimit + step - 1
		if toplimit > pglimit {
			toplimit = pglimit
		}
		go worker(lowlimit, toplimit)
	}

	// wait for workers to send back result
	dcs := make([]*Discuss, 0)
	ers := new(bytes.Buffer)
	for n := 0; n < mxworker; n++ {
		outcome := <-ocqueue
		dcs = append(dcs, outcome.Out...)
		if outcome.Wrong != nil {
			ers.WriteString(outcome.Wrong.Error())
		}
	}

	// return & report error
	if ers.Len() == 0 {
		return dcs, nil
	}
	return dcs, fmt.Errorf("interrupt with errors: %v", ers.String())
}

// discussion in douban group
type Discuss struct {
	Title     string
	Link      string
	ID        string
	Author    string
	Reply     int
	LastReply time.Time
}

// filterDiscuss returns douban discussions parsed from input url which has format like
// 'https://www.douban.com/group/#group_id/discussion?start=#n' and filtered by input func.
func filterDiscuss(url string, filter func(*Discuss) bool) ([]*Discuss, error) {
	// communication check
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

	// filter out discuss links
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

	// get discuss basic info
	var dcs = make([]*Discuss, 0, 25)
	for _, linkNode := range matches {
		var d Discuss
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
		if reply, err := strconv.Atoi(replyStr); err != nil {
			return nil, fmt.Errorf("convert reply-num for '%s': %v", d.Title, err)
		} else {
			d.Reply = reply
		}
		lastReplyNode := replyNode.NextSibling.NextSibling
		lastReplyStr := scrape.Text(lastReplyNode)
		if len(lastReplyStr) == 10 {
			lastReplyStr = fmt.Sprintf("%s 00:00 CST", lastReplyStr)
		} else {
			lastReplyStr = fmt.Sprintf("%d-%s CST", time.Now().Year(), lastReplyStr)
		}
		if lastReply, err := time.Parse("2006-01-02 15:04 MST",
			lastReplyStr); err != nil {
			return nil, fmt.Errorf("convert last-reply-time for '%s': %v", d.Title, err)
		} else {
			d.LastReply = lastReply
		}
		if filter == nil || filter(&d) {
			dcs = append(dcs, &d)
		}
	}
	return dcs, nil
}

// corresponding to json configure file
type Config struct {
	GroupID    string
	MaxPage    int
	MaxWorker  int
	SearchKey  string
	RenderFile string
	SMTPMail   struct {
		Send  bool
		From  string
		To    string
		Token string
	}
}

// load configural parameter from json file
func loadConfig(filePath string) (*Config, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %v", err)
	}
	cfg := new(Config)
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("unmarshal json: %v", err)
	}
	return cfg, nil
}

// load template from plain text file
func loadTempl(filePath string, name string) (*template.Template, error) {
	ftm, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %v", err)
	}
	defer ftm.Close()
	tmpl, err := ioutil.ReadAll(ftm)
	if err != nil {
		return nil, fmt.Errorf("read file: %v", err)
	}
	renderer, err := template.New(name).Parse(string(tmpl))
	if err != nil {
		return nil, fmt.Errorf("parse template: %v", err)
	}
	return renderer, nil
}

// sendMail takes email sender's address, receivers' addresses joined by ';' into one string,
// email client password, email subject and body to send email by SMTP protocol.
func sendMail(from, to, pwd, sub, body string) error {
	domain := from[strings.Index(from, "@")+1:]
	auth := smtp.PlainAuth("", from, pwd, fmt.Sprintf("smtp.%s", domain))
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"Subject: %s\r\n"+
		"\r\n%s\r\n", from, to, sub, body)
	if err := smtp.SendMail(fmt.Sprintf("smtp.%s:25", domain), auth,
		from, strings.Split(to, ";"), []byte(msg)); err != nil {
		return err
	}
	return nil
}
