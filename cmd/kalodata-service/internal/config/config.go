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
		Concurrency int `json:",default=10"`
	}
	Kalodata struct {
		BaseURL          string `json:",default=https://www.kalodata.com"`
		CDNBase          string `json:",default=https://img.kalocdn.com"`
		Cookies          string `json:",optional"` // env KALODATA_COOKIES, one account per line
		CookieTTL        string `json:",default=168h"`
		DailySearchQuota int    `json:",default=10"`
		DailyDetailQuota int    `json:",default=10"`
		Retries          int    `json:",default=3"`
	}
}
