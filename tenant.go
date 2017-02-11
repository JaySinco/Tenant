package main

import (
	"flag"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var keyRegxps []*regexp.Regexp
var GroupFlag = flag.String("Group", "shanghaizufang", "Group name")
var maxFlag = flag.Int("max", 25, "max num of Posts")
var keyFlag = flag.String("key", ".*", "search key(';' separated)")

func main() {
	start := time.Now()
	flag.Parse()
	fmt.Println("======= CONFIG ========")
	fmt.Printf("GROUP ID     => %s\n", *GroupFlag)
	fmt.Printf("Post LIMIT => %d\n", *maxFlag)
	if err := compileKey(); err != nil {
		fmt.Printf("main: %v\n", err)
	}
	fmt.Println("======= RESULT ========")
	if err := (&Group{*GroupFlag, *maxFlag}).ForEach(filter); err != nil {
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

func filter(t Post) error {
	for _, pat := range keyRegxps {
		if pat.Match([]byte(t.Theme)) {
			fmt.Println(t)
		}
	}
	return nil
}
