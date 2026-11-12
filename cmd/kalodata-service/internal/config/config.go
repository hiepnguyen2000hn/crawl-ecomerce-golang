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
		BaseURL string `json:",default=https://www.kalodata.com"`
		CDNBase string `json:",default=https://img.kalocdn.com"`
		// Primary source: kalodata_full_info.json (array of {session_cookie,
		// device_id, proxy}). Env KALODATA_ACCOUNTS_FILE.
		AccountsFile string `json:",optional"`
		// Fallback: env KALODATA_COOKIES, one Cookie header per line (no proxy).
		Cookies          string `json:",optional"`
		// Display locale sent as request headers. The job's country drives the
		// `country` header (selects the market); currency/language are fixed here.
		Currency string `json:",default=USD"`
		Language string `json:",default=en-US"`
		// Keyword search runs through a browser login (Camoufox) because
		// /product/searchList rejects injected-cookie sessions.
		Python           string `json:",default=/home/hiepnv/code/crawl-ecomerce-golang/scripts/camoufox-yolo-captcha/.venv/bin/python"`
		SearchScript     string `json:",default=cmd/kalodata-service/crawler/search.py"`
		CookieTTL        string `json:",default=168h"`
		DailySearchQuota int    `json:",default=10"`
		DailyDetailQuota int    `json:",default=10"`
		Retries          int    `json:",default=3"`
	}
}
