// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf
	Postgres struct {
		DSN string
	}
	RabbitMQ struct {
		URL      string
		Exchange string
	}
	TrendRpc struct {
		Target string
	}
	FbAdsRpc struct {
		Target string
	}
	AmazonRpc struct {
		Target string
	}
	China1688Rpc struct {
		Target string
	}
	RedditRpc struct {
		Target string
	}
	RedditApiRpc struct {
		Target string
	}
	AllegroRpc struct {
		Target string `json:",default=127.0.0.1:8089"`
	}
	LinkedInRpc struct {
		Target string `json:",default=127.0.0.1:8092"`
	}
	KalodataRpc struct {
		Target string `json:",default=127.0.0.1:8093"`
	}
}
