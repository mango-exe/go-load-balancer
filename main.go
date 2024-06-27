package main

import (
	"fmt"
	"net/http/httputil"
	"net/url"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	current int
	mutex   sync.Mutex
)

func proxyRequest(urls []*url.URL) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		currentUrl := roundRobin(urls)
		fmt.Fprintf(gin.DefaultWriter, currentUrl.Host)

		proxy := httputil.NewSingleHostReverseProxy(currentUrl)
		ctx.Request.URL.Host = currentUrl.Host
		ctx.Request.URL.Scheme = currentUrl.Scheme
		ctx.Request.Header.Set("X-Forwarded-Host", ctx.Request.Host)
		ctx.Request.Host = currentUrl.Host

		proxy.ServeHTTP(ctx.Writer, ctx.Request)
	}
}

func roundRobin(urls []*url.URL) *url.URL {
	mutex.Lock()
	defer mutex.Unlock()

	current = (current + 1) % len(urls)
	fmt.Println(current)
	return urls[current]
}

func main() {
	var rawURLs = []string{"http://127.0.0.1:8081", "http://127.0.0.1:8082"}

	var urls []*url.URL

	for _, rawUrl := range rawURLs {
		parsedUrl, err := url.Parse(rawUrl)

		if err != nil {
			fmt.Println(err)
		}
		urls = append(urls, parsedUrl)
	}

	r := gin.Default()

	r.Any("/*path", proxyRequest(urls))

	r.Run()
}
