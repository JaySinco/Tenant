package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
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
	dcs, err := search(cfg.GroupID, cfg.MaxPage, cfg.MaxWorker, cfg.SearchKey)
	if err != nil {
		log.Printf("[ERROR] search: %v", err)
	}
	log.Printf("[INFO ] %d discusses found", len(dcs))
}

// corresponding to json configure file
type Config struct {
	GroupID   string
	MaxPage   int
	MaxWorker int
	SearchKey string
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

// discussion in douban group
type Discuss struct {
	Title     string
	Link      string
	ID        string
	Author    string
	Reply     int
	LastReply time.Time
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
			if dcs, err := collectDiscuss(url, filter); err != nil {
				wrong = err
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
	erm := make(map[string]int)
	for n := 0; n < mxworker; n++ {
		outcome := <-ocqueue
		dcs = append(dcs, outcome.Out...)
		if outcome.Wrong != nil {
			erm[outcome.Wrong.Error()]++
		}
	}

	// return & report error
	if len(erm) == 0 {
		return dcs, nil
	}
	ers := new(bytes.Buffer)
	ern := 0
	for s, n := range erm {
		ern += n
		ers.WriteString(fmt.Sprintf("%s[%d times];", s, n))
	}
	return dcs, fmt.Errorf("%d/%d workers error interrupt: %v",
		ern, mxworker, ers.String())
}

// discussGet returns douban discussions parsed from input url which has format like
// 'https://www.douban.com/group/#group_id/discussion?start=#n' and filtered by input func.
func collectDiscuss(url string, filter func(*Discuss) bool) ([]*Discuss, error) {
	// communication check
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("unable connect to network")
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
		return nil, fmt.Errorf("response html title is '豆瓣'")
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
