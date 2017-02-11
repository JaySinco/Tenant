package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func main() {
	if err := (&Group{"shanghaizufang", 50}).ForEach(debug); err != nil {
		fmt.Printf("main: %v\n", err)
	}
}

func debug(p Post) error {
	fmt.Println(p)
	return nil
}

type Post struct {
	Title     string
	Link      string
	Author    string
	Reply     int
	LastReply time.Time
	Created   time.Time
	Content   string
}

type Group struct {
	Name     string
	Capacity int
}

func (g *Group) ForEach(handle func(p Post) error) error {
	page := g.Capacity / 25
	for n := 0; n < page; n++ {
		url := fmt.Sprintf("https://www.douban.com/group/%s/discussion?start=%d", g.Name, n*25+1)
		if err := g.eachPage(url, handle); err != nil {
			return fmt.Errorf("posts in %s: %v", url, err)
		}
	}
	return nil
}

func (g *Group) eachPage(url string, handle func(p Post) error) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	root, err := html.Parse(resp.Body)
	if err != nil {
		return err
	}
	matches := scrape.FindAll(root, postMatcher)
	if len(matches) == 0 {
		return fmt.Errorf("match zero post =>\n%s", node2String(root))
	}
	for _, linkNode := range matches {
		if post, err := fromMatcher(linkNode); err != nil {
			return fmt.Errorf("matches2post: %v", err)
		} else {
			if err := handle(post); err != nil {
				return fmt.Errorf("handle %v: %v", post, err)
			}
		}
	}
	return nil
}

func postMatcher(n *html.Node) bool {
	if n.DataAtom == atom.A && n.Parent != nil && n.Parent.DataAtom == atom.Td &&
		scrape.Attr(n.Parent, "class") == "title" {
		return true
	}
	return false
}

func fromMatcher(linkNode *html.Node) (Post, error) {
	link := scrape.Attr(linkNode, "href")
	authorNode := linkNode.Parent.NextSibling.NextSibling
	replyNode := authorNode.NextSibling.NextSibling
	replyStr := scrape.Text(replyNode)
	if replyStr == "" {
		replyStr = "0"
	}
	reply, err := strconv.Atoi(replyStr)
	if err != nil {
		return Post{}, fmt.Errorf("convert reply num: %v", err)
	}
	lastReplyNode := replyNode.NextSibling.NextSibling
	lastReply, err := time.Parse("2006-01-02 15:04 MST", "2017-"+scrape.Text(lastReplyNode)+" CST")
	if err != nil {
		return Post{}, fmt.Errorf("convert last reply time: %v", err)
	}
	content, created, err := getDetail(link)
	if err != nil {
		return Post{}, fmt.Errorf("get post detail from %s: %v", link, err)
	}
	return Post{
		Title:     scrape.Text(linkNode),
		Link:      link,
		Author:    scrape.Text(authorNode),
		Reply:     reply,
		LastReply: lastReply,
		Content:   content,
		Created:   created,
	}, nil
}

func getDetail(url string) (content string, created time.Time, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return content, created, err
	}
	defer resp.Body.Close()
	root, err := html.Parse(resp.Body)
	if err != nil {
		return content, created, err
	}
	if createdNode, ok := scrape.Find(root, scrape.ByClass("color-green")); !ok {
		return content, created, fmt.Errorf("find no created time element(class=color-green) =>\n%s", node2String(root))
	} else {
		created, err = time.Parse("2006-01-02 15:04:05 MST", scrape.Text(createdNode)+" CST")
		if err != nil {
			return content, created, fmt.Errorf("convert created time: %v", err)
		}
	}
	if contentNode, ok := scrape.Find(root, scrape.ById("link-report")); !ok {
		return content, created, fmt.Errorf("find no content element(id=link-report) =>\n%s", node2String(root))
	} else {
		return scrape.Text(contentNode), created, nil
	}
}

func node2String(n *html.Node) string {
	var web bytes.Buffer
	html.Render(&web, n)
	return web.String()
}
