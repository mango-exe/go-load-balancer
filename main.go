package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var (
	current int
	mutex   sync.Mutex
)

type StickySession struct {
	cookies []*http.Cookie
}

func proxyRequest(urls []*url.URL) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		existingCookie, err := ctx.Request.Cookie("sticky-session")

		currentUrl := roundRobin(urls)

		if err != nil {
			id := uuid.New()
			cookieValue := fmt.Sprintf("%s@%s", id, currentUrl.String())
			encodedValue := base64.RawStdEncoding.EncodeToString([]byte(cookieValue))

			ctx.SetCookie("sticky-session", encodedValue, 3600, "", "", false, true)
		} else {
			decodedCookieBytes, err := base64.RawStdEncoding.DecodeString(existingCookie.Value)

			if err != nil {
				fmt.Println(err)
			}

			splitCookieValue := strings.Split(string(decodedCookieBytes), "@")

			if len(splitCookieValue) == 2 {
				_, rawUrl := splitCookieValue[0], splitCookieValue[1]

				currentUrl, err = url.Parse(rawUrl)

				if err != nil {
					fmt.Println(err)
				}
			}
		}

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
