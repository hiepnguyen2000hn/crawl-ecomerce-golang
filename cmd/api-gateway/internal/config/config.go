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
}
