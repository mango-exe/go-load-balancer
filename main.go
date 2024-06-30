package main

import "github.com/mango-exe/go-load-balancer/lib"

func main() {
	loadBalancer := lib.BuildLoadBalancer()
	loadBalancer.Run()
}
