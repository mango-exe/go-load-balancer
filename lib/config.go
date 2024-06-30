package lib

import (
	"fmt"
	"io/ioutil"
	"slices"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Servers        []string `yaml:"servers"`
	StickySessions bool     `yaml:"stickySessions"`
	HealthCheck    bool     `yaml:"healthCheck"`
	Tls            bool     `yaml:"tls"`
	RateLimit      bool     `yaml:"rateLimit"`
	Type           string   `yaml:"type"`
}

func loadConfig() (*Config, error) {
	data, err := ioutil.ReadFile("./config.yaml")

	if err != nil {
		return nil, err
	}

	var config Config

	err = yaml.Unmarshal(data, &config)

	if err != nil {
		return nil, err
	}

	allowedLoadBalancerTypes := []string{"roundrobin", "leastconnections"}
	validType := slices.Contains(allowedLoadBalancerTypes, config.Type)

	if !validType {
		fmt.Println("Invalid load balancer type")
		return nil, nil
	}

	return &config, nil
}
