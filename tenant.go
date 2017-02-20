package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

var groupFlag = flag.String("g", "shanghaizufang", "group name")
var maxFlag = flag.Int("n", 25, "max num of posts")
var keyFlag = flag.String("k",
	"真北路;大渡河;金沙江路;娄山关路;威宁路;北新泾;淞虹路;中山公园;延安西路;虹桥路;宜山路;曹杨路;上海体育馆;桂林路;漕河泾开发区;合川路",
	"search key(';' separated)")

var keyRegxps []*regexp.Regexp
var dupCheck = make(map[string]bool)

func main() {
	start := time.Now()
	flag.Parse()
	if regs, err := compileKey(*keyFlag); err != nil {
		fmt.Printf("main: %v\n", err)
		os.Exit(-1)
	} else {
		keyRegxps = regs
	}
	fmt.Println("=============== https://www.douban.com/group/topic ===============")
	group := Group{*groupFlag, *maxFlag}
	if err := group.EachPost(filter); err != nil {
		fmt.Printf("main: %v\n", err)
		os.Exit(-1)
	}
	fmt.Println("========== EXIT ==========")
	fmt.Println(time.Since(start))
}

func compileKey(keyStr string) ([]*regexp.Regexp, error) {
	keyWords := strings.Split(keyStr, ";")
	regs := make([]*regexp.Regexp, 0, len(keyWords))
	for _, kw := range keyWords {
		if pat, err := regexp.Compile(kw); err != nil {
			return nil, fmt.Errorf("compile key '%s': %v", kw, err)
		} else {
			regs = append(regs, pat)
		}
	}
	return regs, nil
}

func filter(p *Post) error {
	titleByte := []byte(p.Title)
	for _, pat := range keyRegxps {
		if pat.Match(titleByte) {
			if !dupCheck[p.Link] {
				fmt.Println(renderPost(p))
				dupCheck[p.Link] = true
			}
			return nil
		}
	}
	return nil
}

func renderPost(p *Post) string {
	link := strings.TrimRight(p.Link, "/")
	link = link[strings.LastIndex(link, "/"):]
	return fmt.Sprintf("%5d  %s(%s)", p.Reply,
		strings.Replace(p.ShortTitle, "\n", "", -1), link)
}
