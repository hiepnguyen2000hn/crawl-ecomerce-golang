package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	Postgres struct {
		DSN string
	}
	RabbitMQ struct {
		URL         string
		Exchange    string
		Concurrency int
	}
	Pipiads struct {
		BaseURL      string
		AccountsFile string
		Retries      int
	}
}
