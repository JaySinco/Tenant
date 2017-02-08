package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var keyRegxps []*regexp.Regexp
var forumFlag = flag.String("forum", "shanghaizufang", "forum name")
var maxFlag = flag.Int("max", 25, "max num of threads")
var keyFlag = flag.String("key", ".*", "search key(';' separated)")

func main() {
	start := time.Now()
	flag.Parse()
	fmt.Println("======= CONFIG ========")
	fmt.Printf("GROUP ID     => %s\n", *forumFlag)
	fmt.Printf("THREAD LIMIT => %d\n", *maxFlag)
	if err := compileKey(); err != nil {
		fmt.Printf("main: %v\n", err)
	}
	fmt.Println("======= RESULT ========")
	if err := (&Forum{*forumFlag, *maxFlag}).ForEach(filter); err != nil {
		fmt.Printf("main: %v\n", err)
	}
	fmt.Println("======= EXIT ==========")
	fmt.Println(time.Since(start))
}

func compileKey() error {
	keyWords := strings.Split(*keyFlag, ";")
	keyRegxps = make([]*regexp.Regexp, 0, len(keyWords))
	for _, kw := range keyWords {
		if pat, err := regexp.Compile(kw); err != nil {
			return fmt.Errorf("compile key '%s': %v", kw, err)
		} else {
			fmt.Printf("KEY ADDED    => '%s'\n", kw)
			keyRegxps = append(keyRegxps, pat)
		}
	}
	return nil
}

func filter(t Thread) error {
	for _, pat := range keyRegxps {
		if pat.Match([]byte(t.Theme)) {
			fmt.Println(t)
		}
	}
	return nil
}

type Thread struct {
	Theme, Link, Author, Reply, LastReply string
}

func (t Thread) String() string {
	const MAX_WORD int = 45
	t.Link = strings.TrimRight(t.Link, "/")
	t.Link = t.Link[strings.LastIndex(t.Link, "/")+1:]
	var suffix string
	if len([]rune(t.Theme)) > MAX_WORD {
		suffix = "..."
	}
	return fmt.Sprintf("%11s [%-8s] %.*s%s", t.LastReply, t.Link, MAX_WORD, t.Theme, suffix)
}

type Forum struct {
	Name     string
	Capacity int
}

func (f *Forum) ForEach(handle func(t Thread) error) error {
	page := f.Capacity / 25
	for n := 0; n < page; n++ {
		url := fmt.Sprintf("https://www.douban.com/group/%s/discussion?start=%d", f.Name, n*25+1)
		resp, err := http.Get(fmt.Sprintf(url))
		if err != nil {
			return fmt.Errorf("get douban group <%s/p%d>: %v", f.Name, n+1, err)
		}
		defer resp.Body.Close()
		threads, err := f.Extract(resp.Body)
		if err != nil {
			return fmt.Errorf("extract threads from %s: %v", url, err)
		}
		for _, tp := range threads {
			if err := handle(tp); err != nil {
				return fmt.Errorf("handle thread %v: %v", tp, err)
			}
		}
	}
	return nil
}

func (f *Forum) Extract(source io.Reader) ([]Thread, error) {
	root, err := html.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse source as html: %v", err)
	}
	matches := scrape.FindAll(root, func(n *html.Node) bool {
		if n.DataAtom == atom.A && n.Parent != nil && n.Parent.DataAtom == atom.Td &&
			scrape.Attr(n.Parent, "class") == "title" {
			return true
		}
		return false
	})
	if len(matches) == 0 {
		return nil, fmt.Errorf("extract zero thread from source")
	}
	threads := make([]Thread, 0, 25)
	for _, themeLink := range matches {
		author := themeLink.Parent.NextSibling.NextSibling
		reply := author.NextSibling.NextSibling
		lastReply := reply.NextSibling.NextSibling
		threads = append(threads, Thread{
			Theme:     strings.Replace(scrape.Attr(themeLink, "title"), "\n", "", -1),
			Link:      scrape.Attr(themeLink, "href"),
			Author:    scrape.Text(author),
			Reply:     scrape.Text(reply),
			LastReply: scrape.Text(lastReply),
		})
	}
	return threads, nil
}
