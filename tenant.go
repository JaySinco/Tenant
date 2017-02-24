package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"regexp"
	"strings"
)

var groupFlag = flag.String("g", "shanghaizufang", "group name")
var maxFlag = flag.Int("n", 25, "max num of posts")
var keyFlag = flag.String("k",
	"真北路大渡河|金沙江路|娄山关路|威宁路|北新泾|淞虹路|中山公园|延安西路|虹桥路|"+
		"曹杨路|上海体育馆|桂林路|漕河泾开发区|合川路|伊犁路|宋园路|水城路|龙溪路|"+
		"宜山路|上海动物园|龙柏新村|紫藤路|虹桥1号航站楼",
	"search key pattern")

var pwdFlag = flag.String("p", "", "email password")
var toFlag = flag.String("t", "jaysinco@163.com", "email receivers sep by ';'")

var keyRegxp *regexp.Regexp
var postsPicked = make([]*Post, 0, 10)
var dupCheck = make(map[string]bool)

const postTmpl = `
<table border="1">
	{{range .}}
	<tr>
		<td>{{.Reply}}</td>
		<td><a href="{{.Link}}">{{.ShortTitle}}</a></td>
	</tr>
	{{end}}
</table>`

func main() {
	flag.Parse()
	keyRegxp = regexp.MustCompile(*keyFlag)
	if err := (&Group{*groupFlag, *maxFlag}).EachPost(filter); err != nil {
		log.Fatalf("main: each post: %v\n", err)
	}
	tmpl, err := template.New("Post Report").Parse(postTmpl)
	if err != nil {
		log.Fatalf("main: parse template: %v\n", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, postsPicked); err != nil {
		log.Fatalf("main: render template: %v\n", err)
	}
	if err := (&Mail{*pwdFlag, "豆瓣租房报告", *toFlag,
		buf.String()}).Send(); err != nil {
		log.Fatalf("main: %v\n", err)
	}
}

func filter(p *Post) error {
	if keyRegxp.Match([]byte(p.Title)) && !dupCheck[p.ID] {
		postsPicked = append(postsPicked, p)
		dupCheck[p.ID] = true
	}
	return nil
}

type Mail struct {
	Password string
	Subject  string
	To       string
	Body     string
}

func (e *Mail) Send() error {
	const host = "smtp.qq.com:25"
	const account = "zxk156@qq.com"
	auth := smtp.PlainAuth("", account, e.Password,
		strings.Split(host, ":")[0])
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"Subject: %s\r\n"+
		"\r\n%s\r\n", account, e.To, e.Subject, e.Body)
	if err := smtp.SendMail(host, auth, account,
		strings.Split(e.To, ";"), []byte(msg)); err != nil {
		return fmt.Errorf("send email using %s: %v", account, err)
	}
	return nil
}
