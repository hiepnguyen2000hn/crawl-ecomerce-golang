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
		Concurrency int `json:",default=1"`
	}
	Allegro struct {
		Python         string `json:",default=/home/hiepnv/code/crawl-ecomerce-golang/scripts/camoufox-yolo-captcha/.venv/bin/python"`
		Script         string `json:",default=cmd/allegro-service/crawler/fetch.py"`
		ProfileDir     string `json:",optional"`
		Proxy          string `json:",optional"`
		ProxyFile      string `json:",optional"`
		Xvfb           bool   `json:",default=false"`
		TimeoutSeconds int    `json:",default=300"`
	}
}
