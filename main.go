package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/robfig/cron/v3"
)

type Config struct {
	ProxyPort int
	Listen    string
	Cron      string
	UA        string
}

var defaultConfig = Config{
	ProxyPort: 8080,
	Listen:    "127.0.0.1:1234",
	Cron:      "*/30 6-21 * * *",
	UA:        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36 Edg/116.0.1938.62",
}

type LoginInfo struct {
	Cookie string
	Token  string
}

type Error struct {
	Code    int
	Err     error
	Message string
}

var (
	loginInfo     LoginInfo
	studentCardId string
	pwd           string
)

func main() {
	studentCardId = os.Getenv("STUDENT_CARD_ID")
	pwd = os.Getenv("STUDENT_CARD_ID_PASSWORD")
	if studentCardId == "" || pwd == "" {
		log.Fatalf("[ERROR] STUDENT_CARD_ID or STUDENT_CARD_ID_PASSWORD is empty, please set environment variables\n")
	}

	cron := cron.New()
	cron.AddFunc("*/30 6-21 * * *", func() {
		getCookieAndToken()
	})
	cron.Start()
	go getCookieAndToken()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		res, err := json.Marshal(loginInfo)
		if err != nil {
			res, _ = json.Marshal(Error{
				Code:    500,
				Err:     err,
				Message: "error",
			})
			fmt.Fprint(w, string(res))
			return
		}
		if loginInfo.Cookie == "" || loginInfo.Token == "" {
			res, _ = json.Marshal(Error{
				Code:    http.StatusInternalServerError,
				Err:     fmt.Errorf("Cookie or Token is empty"),
				Message: "Cookie or Token is empty",
			})
			fmt.Fprintf(w, string(res))
			return
		}
		fmt.Fprint(w, string(res))
	})
	go http.ListenAndServe(defaultConfig.Listen, nil)
	select {}
}

func getCookieAndToken() {
	proxy := goproxy.NewProxyHttpServer()
	c := make(chan struct{})
	proxy.Verbose = true
	proxy.OnRequest().DoFunc(
		func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			if r.URL.Host != "libzwyy.jlu.edu.cn" {
				return nil, nil
			}
			if r.Header.Get("Cookie") != "" && r.Header.Get("Token") != "" {
				loginInfo = LoginInfo{
					Cookie: r.Header.Get("Cookie"),
					Token:  r.Header.Get("Token"),
				}
				close(c)
			}
			return r, nil
		})
	go http.ListenAndServe(fmt.Sprintf(":%d", defaultConfig.ProxyPort), proxy)

	l := launcher.New()
	l = l.Set(flags.ProxyServer, fmt.Sprintf("http://127.0.0.1:%d", defaultConfig.ProxyPort))
	controlURL, _ := l.Launch()
	browser := rod.New().ControlURL(controlURL).MustConnect()

	page := browser.MustPage("http://libzwyy.jlu.edu.cn/#/ic/home")
	page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36 Edg/116.0.1938.62"})

	go func() {
		time.Sleep(20 * time.Second)
		select {
		case <-c:
			return
		}
	}()
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(1) > div > div > input").MustInput(studentCardId)
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(1) > div > div > input").MustMoveMouseOut()
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(2) > div > div > input").MustClick().MustInput(pwd)
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(2) > div > div > input").MustMoveMouseOut()
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(3) > div > button").MustClick()
	<-c
	page.MustClose()
	return
}
