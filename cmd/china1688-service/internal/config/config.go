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
		// How many jobs this service handles at once. The real ceiling is the
		// global Apify slot pool, not this -- raising it past the pool just
		// makes jobs queue inside Acquire instead of in RabbitMQ.
		Concurrency int `json:",default=10"`
	}
	Apify struct {
		ApiToken string
		ActorID  string
		BaseURL  string
		// Concurrent runs allowed per Apify account. Apify's free plan is 5.
		MaxConcurrencyPerToken int `json:",default=5"`
	}
}
