package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	Postgres struct {
		DSN string
	}
	RabbitMQ struct {
		URL      string
		Exchange string
	}
	Reddit struct {
		ClientID     string
		ClientSecret string
		UserAgent    string
		AuthURL      string
		ApiURL       string
	}
}
