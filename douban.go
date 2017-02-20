package main

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type Group struct {
	Name     string
	Capacity int
}

func (g *Group) EachPost(handle func(base *Post) error) error {
	page := g.Capacity / 25
	for n := 0; n < page; n++ {
		url := fmt.Sprintf("https://www.douban.com/group/%s/discussion?start=%d",
			g.Name, n*25+1)
		if err := eachPage(url, handle); err != nil {
			return fmt.Errorf("posts in %s: %v", url, err)
		}
	}
	return nil
}

func eachPage(url string, handle func(base *Post) error) error {
	root, err := doubanGet(url)
	if err != nil {
		return fmt.Errorf("get douban: %v", err)
	}
	matches := scrape.FindAll(root, postLinkMatcher)
	if len(matches) == 0 {
		return NoneElementErr("/?postLinkMatcher", root)
	}
	for _, linkNode := range matches {
		var base Post
		if err := base.getBasic(linkNode); err != nil {
			return fmt.Errorf("get post basic: %v", err)
		} else {
			if err := handle(&base); err != nil {
				return fmt.Errorf("handle %#v: %v", base, err)
			}
		}
	}
	return nil
}

func doubanGet(url string) (*html.Node, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http status code: %s", resp.Status)
	}
	root, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}
	if titleNode, ok := scrape.Find(root, scrape.ByTag(atom.Title)); ok &&
		scrape.Text(titleNode) == "豆瓣" {
		return nil, fmt.Errorf("response html title is '豆瓣', need to login")
	}
	return root, nil
}

func postLinkMatcher(n *html.Node) bool {
	if n.DataAtom == atom.A && n.Parent != nil && n.Parent.DataAtom == atom.Td &&
		scrape.Attr(n.Parent, "class") == "title" {
		return true
	}
	return false
}

func NoneElementErr(desc string, n *html.Node) error {
	var buf bytes.Buffer
	html.Render(&buf, n)
	return fmt.Errorf("select no '%s' =>\n%s\n",
		desc, strings.Repeat("*", 80), buf.String())
}

type Post struct {
	ShortTitle string
	Title      string
	Link       string
	Author     string
	Reply      int
	LastReply  time.Time
	Created    time.Time // detail part begin...
	Content    string
	Favor      int
}

func (p *Post) getBasic(linkNode *html.Node) error {
	p.Link = scrape.Attr(linkNode, "href")
	p.Title = scrape.Attr(linkNode, "title")
	p.ShortTitle = scrape.Text(linkNode)
	authorNode := linkNode.Parent.NextSibling.NextSibling
	p.Author = scrape.Text(authorNode)
	replyNode := authorNode.NextSibling.NextSibling
	replyStr := scrape.Text(replyNode)
	if replyStr == "" {
		replyStr = "0"
	}
	if reply, err := strconv.Atoi(replyStr); err != nil {
		return fmt.Errorf("convert reply-num: %v", err)
	} else {
		p.Reply = reply
	}
	lastReplyNode := replyNode.NextSibling.NextSibling
	if lastReply, err := time.Parse("2006-01-02 15:04 MST",
		"2017-"+scrape.Text(lastReplyNode)+" CST"); err != nil {
		return fmt.Errorf("convert last-reply-time: %v", err)
	} else {
		p.LastReply = lastReply
	}
	return nil
}

var favorRegexp *regexp.Regexp = regexp.MustCompile(`(\d+)人\s*喜欢`)

func (p *Post) GetDetail() error {
	root, err := doubanGet(p.Link)
	if err != nil {
		return fmt.Errorf("get douban: %v", err)
	}
	if createdNode, ok := scrape.Find(root, scrape.ByClass("color-green")); !ok {
		return NoneElementErr(".color-green", root)
	} else {
		if created, err := time.Parse("2006-01-02 15:04:05 MST",
			scrape.Text(createdNode)+" CST"); err != nil {
			return fmt.Errorf("convert created-time: %v", err)
		} else {
			p.Created = created
		}
	}
	if contentNode, ok := scrape.Find(root, scrape.ById("link-report")); !ok {
		return NoneElementErr("#link-report", root)
	} else {
		p.Content = scrape.Text(contentNode)
	}
	if favorNode, ok := scrape.Find(root, scrape.ByClass("fav-num")); !ok {
		p.Favor = 0
	} else {
		favstr := scrape.Text(favorNode)
		sub := favorRegexp.FindSubmatch([]byte(favstr))
		if len(sub) < 2 {
			return fmt.Errorf("match none digit in '%s'", favstr)
		}
		if favor, err := strconv.Atoi(string(sub[1])); err != nil {
			return fmt.Errorf("convert favor-num: %v", err)
		} else {
			p.Favor = favor
		}
	}
	return nil
}
