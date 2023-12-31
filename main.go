package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

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

type service struct {
	loginInfo     LoginInfo
	loginInfoChan chan struct{}
	wg            *sync.WaitGroup
	browser       *rod.Browser
	logger        *slog.Logger
}

var defaultConfig = Config{
	ProxyPort: 8000,
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
	studentCardId string
	pwd           string
)

func main() {
	studentCardId = os.Getenv("STUDENT_CARD_ID")
	pwd = os.Getenv("STUDENT_CARD_ID_PASSWORD")
	if studentCardId == "" || pwd == "" {
		slog.Error("[ERROR] STUDENT_CARD_ID or STUDENT_CARD_ID_PASSWORD is empty, please set environment variables\n")
		os.Exit(1)
	}

	l := launcher.New()
	l = l.Set(flags.ProxyServer, fmt.Sprintf("http://127.0.0.1:%d", defaultConfig.ProxyPort))
	controlURL, _ := l.Launch()

	s := service{
		loginInfo:     LoginInfo{},
		loginInfoChan: make(chan struct{}),
		wg:            &sync.WaitGroup{},
		browser:       rod.New().ControlURL(controlURL).MustConnect(),
		logger:        slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
	s.wg.Add(1)
	go s.getCookieAndToken()
	go s.startProxy()
	s.logger.Info("starting server")
	go s.startServer()
	s.wg.Wait()

	cron := cron.New()
	cron.AddFunc("*/5 6-21 * * *", func() {
		s.logger.Info("refreshing cookie and token")
		s.getCookieAndToken()
	})
	cron.Start()
	s.logger.Info("cron started")

	select {}
}

func (s *service) startServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		res, err := json.Marshal(s.loginInfo)
		if err != nil {
			res, _ = json.Marshal(Error{
				Code:    500,
				Err:     err,
				Message: "error",
			})
			fmt.Fprint(w, string(res))
			s.logger.Error("error: %v", err)
			return
		}
		if s.loginInfo.Cookie == "" || s.loginInfo.Token == "" {
			res, _ = json.Marshal(Error{
				Code:    http.StatusInternalServerError,
				Err:     fmt.Errorf("Cookie or Token is empty"),
				Message: "Cookie or Token is empty",
			})
			fmt.Fprintf(w, string(res))
			s.logger.Error("error: %v", err)
			return
		}
		fmt.Fprint(w, string(res))
	})
	go http.ListenAndServe(defaultConfig.Listen, nil)
	s.logger.Info("server start at %s", defaultConfig.Listen)
}

func (s *service) startProxy() {
	defer s.wg.Done()

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true
	proxy.OnRequest().DoFunc(
		func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			if r.URL.Host != "libzwyy.jlu.edu.cn" {
				return nil, nil
			}
			if r.Header.Get("Cookie") != "" && r.Header.Get("Token") != "" {
				// get loginInfo here, if got, inform getCookieAndToken() to exit
				s.loginInfo = LoginInfo{
					Cookie: r.Header.Get("Cookie"),
					Token:  r.Header.Get("Token"),
				}
				s.logger.Info("loginInfo: %v", s.loginInfo)
				s.loginInfoChan <- struct{}{}
			}
			return r, nil
		})
	go http.ListenAndServe(fmt.Sprintf(":%d", defaultConfig.ProxyPort), proxy)
	s.logger.Info("proxy start at %d", defaultConfig.ProxyPort)
}

func (s *service) getCookieAndToken() {
	s.wg.Wait()
	page := s.browser.MustPage("http://libzwyy.jlu.edu.cn/#/ic/home")
	page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36 Edg/116.0.1938.62"})
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(1) > div > div > input").MustInput(studentCardId)
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(1) > div > div > input").MustMoveMouseOut()
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(2) > div > div > input").MustClick().MustInput(pwd)
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(2) > div > div > input").MustMoveMouseOut()
	page.MustElement("#app > div.container > div.login-wrapp > div > div.content > form > div:nth-child(3) > div > button").MustClick()
	s.logger.Info("login success")
	<-s.loginInfoChan
	page.MustClose()
	return
}
