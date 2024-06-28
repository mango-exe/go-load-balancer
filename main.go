package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

/*
 TODO:
 - health checks for servers [x]
 - handle tls connections for servers [x]
 - rate limit []
 - least connections load balance algorithm [x]
 - command line configuration for load balancer []
*/

type LoadBalancerType int

const (
	RoundRobin = iota
	LeastConnections
)

type ServerInfo struct {
	url         *url.URL
	connections int
}

type LoadBalancer struct {
	ctx                  *gin.Context
	servers              map[int]*ServerInfo
	current              int
	currentUrl           *url.URL
	stickySessionEnabled bool
	healthChecksEnabled  bool
	tlsEnabled           bool
	rateLimitEnabled     bool
	loadBalancerType     LoadBalancerType
}

func (l *LoadBalancer) stickySession() {
	existingCookie, err := l.ctx.Request.Cookie("sticky-session")

	currentUrl := l.balanceRequest()

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
			l.balanceRequest()
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

func (l *LoadBalancer) leastConnections() *url.URL {
	minConnections := int(^uint(0) >> 1)
	var selectedUrl *url.URL
	var currentKey int

	for key, server := range l.servers {
		if server.connections < minConnections {
			minConnections = server.connections
			selectedUrl = server.url
			currentKey = key
		}
	}

	l.servers[currentKey].connections++
	return selectedUrl
}

func (l *LoadBalancer) roundRobin() *url.URL {
	l.current = (l.current + 1) % len(l.servers)
	return l.servers[l.current].url
}

func (l *LoadBalancer) balanceRequest() *url.URL {
	switch l.loadBalancerType {
	case RoundRobin:
		return l.roundRobin()
	case LeastConnections:
		return l.leastConnections()
	}

	return nil
}

func (l *LoadBalancer) parseUrls(rawUrls []string) {
	var wg sync.WaitGroup
	for idx, rawUrl := range rawUrls {
		wg.Add(1)
		go func(idx int, rawUrl string) {
			defer wg.Done()
			parsedUrl, err := url.Parse(rawUrl)

			if err != nil {
				fmt.Println(err)
			}
			serverInfo := ServerInfo{
				url: parsedUrl,
			}
			l.servers[idx] = &serverInfo
		}(idx, rawUrl)
	}
	wg.Wait()
}

func (l *LoadBalancer) handleRequest() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		l.ctx = ctx

		if l.rateLimitEnabled {
			limit := l.rateLimit()
			fmt.Println(limit)

			if limit {
				ctx.JSON(http.StatusTooManyRequests, gin.H{"message": "Too many requests"})
				return
			}
		}

		if l.stickySessionEnabled {
			l.stickySession()
		} else {
			l.currentUrl = l.balanceRequest()
		}

		l.proxyRequest()
	}
}

func (l *LoadBalancer) healthCheck() {
	var wg sync.WaitGroup
	for _, server := range l.servers {
		wg.Add(1)

		go func(server *ServerInfo) {
			defer wg.Done()
			healthCheckUrl := fmt.Sprintf("%s/health-check", server.url.String())
			response, err := http.Get(healthCheckUrl)
			var message string
			if err != nil || response.StatusCode != http.StatusOK {
				message = fmt.Sprintf("Server %s could not respond %s", server.url.String(), err)
				fmt.Println(message)
			} else {
				message = fmt.Sprintf("Server %s status: %s", server.url.String(), response.Status)
				fmt.Println(message)
			}
		}(server)
	}
	wg.Wait()
}

func (l *LoadBalancer) runHealthChecks() {
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				l.healthCheck()
			}
		}
	}()
}

const requestLimit = 100
const rateLimitWindow = time.Minute

var requestCounts = make(map[string]int)
var requestTimestamps = make(map[string]time.Time)

func (l *LoadBalancer) rateLimit() bool {
	clientIp := l.ctx.ClientIP()
	now := time.Now()

	if requestCounts[clientIp] == 0 {
		requestTimestamps[clientIp] = now
	}

	elapsed := now.Sub(requestTimestamps[clientIp])
	if elapsed > rateLimitWindow {
		requestCounts[clientIp] = 0
		requestTimestamps[clientIp] = now
	}

	requestCounts[clientIp]++

	if requestCounts[clientIp] > requestLimit {
		return true
	}
	return false
}

func (l *LoadBalancer) run() {
	var rawURLs = []string{"http://127.0.0.1:8081", "http://127.0.0.1:8082"}
	l.parseUrls(rawURLs)

	if l.healthChecksEnabled {
		l.runHealthChecks()
	}

	r := gin.Default()

	r.Any("/*path", l.handleRequest())

	if l.tlsEnabled {
		certFilePath := "./certs/cert.pem"
		keyFilePath := "./certs/key.pem"

		log.Fatal(r.RunTLS(":8443", certFilePath, keyFilePath))

	} else {
		r.Run()
	}
}

func main() {

	loadBalancer := LoadBalancer{
		stickySessionEnabled: true,
		healthChecksEnabled:  true,
		tlsEnabled:           false,
		rateLimitEnabled:     true,
		servers:              make(map[int]*ServerInfo),
		loadBalancerType:     LeastConnections,
	}

	loadBalancer.run()
}
