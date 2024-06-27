package main

import (
	"encoding/base64"
	"fmt"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

/*
 TODO:
 - health checks for servers
 - handle tls connections for servers
 - rate limit
 - least connections load balance algorithm
 - command line configuration for load balancer
*/

type LoadBalancer struct {
	ctx                  *gin.Context
	urls                 []*url.URL
	current              int
	currentUrl           *url.URL
	stickySessionEnabled bool
}

func (l *LoadBalancer) stickySession() {
	existingCookie, err := l.ctx.Request.Cookie("sticky-session")

	currentUrl := l.roundRobin()

	if err != nil {
		id := uuid.New()
		cookieValue := fmt.Sprintf("%s@%s", id, currentUrl.String())
		encodedValue := base64.RawStdEncoding.EncodeToString([]byte(cookieValue))

		l.ctx.SetCookie("sticky-session", encodedValue, 3600, "", "", false, true)
	} else {
		decodedCookieBytes, err := base64.RawStdEncoding.DecodeString(existingCookie.Value)

		if err != nil {
			fmt.Println(err)
		}

		splitCookieValue := strings.Split(string(decodedCookieBytes), "@")

		if len(splitCookieValue) == 2 {
			_, rawUrl := splitCookieValue[0], splitCookieValue[1]

			parsedUrl, err := url.Parse(rawUrl)

			if err != nil {
				fmt.Println(err)
			}

			l.currentUrl = parsedUrl
		} else {
			l.roundRobin()
		}
	}
}

func (l *LoadBalancer) proxyRequest() {
	proxy := httputil.NewSingleHostReverseProxy(l.currentUrl)
	l.ctx.Request.URL.Host = l.currentUrl.Host
	l.ctx.Request.URL.Scheme = l.currentUrl.Scheme
	l.ctx.Request.Header.Set("X-Forwarded-Host", l.ctx.Request.Host)
	l.ctx.Request.Host = l.currentUrl.Host

	proxy.ServeHTTP(l.ctx.Writer, l.ctx.Request)
}

func (l *LoadBalancer) roundRobin() *url.URL {
	l.current = (l.current + 1) % len(l.urls)
	return l.urls[l.current]
}

func (l *LoadBalancer) parseUrls(rawUrls []string) {
	var urls []*url.URL

	for _, rawUrl := range rawUrls {
		parsedUrl, err := url.Parse(rawUrl)

		if err != nil {
			fmt.Println(err)
		}
		urls = append(urls, parsedUrl)
	}

	l.urls = urls
}

func (l *LoadBalancer) handleRequest() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		l.ctx = ctx

		if l.stickySessionEnabled {
			l.stickySession()
		} else {
			l.currentUrl = l.roundRobin()
		}

		l.proxyRequest()
	}
}

func main() {
	var rawURLs = []string{"http://127.0.0.1:8081", "http://127.0.0.1:8082"}

	loadBalancer := LoadBalancer{
		stickySessionEnabled: true,
	}

	loadBalancer.parseUrls(rawURLs)

	r := gin.Default()

	r.Any("/*path", loadBalancer.handleRequest())

	r.Run()
}
