package svc

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/kalodata-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/kalodata"
)

type ServiceContext struct {
	Config config.Config
	DB     *sql.DB
	Client kalodata.Client
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}

	ttl, err := time.ParseDuration(c.Kalodata.CookieTTL)
	if err != nil {
		panic(fmt.Errorf("kalodata-service: CookieTTL=%q invalid: %w", c.Kalodata.CookieTTL, err))
	}

	registry := kalodata.NewCookieRegistry(conn, ttl, c.Kalodata.DailySearchQuota, c.Kalodata.DailyDetailQuota)

	// Primary: accounts file (session_cookie + device_id + proxy per account).
	if c.Kalodata.AccountsFile != "" {
		accounts, err := kalodata.LoadAccounts(c.Kalodata.AccountsFile)
		if err != nil {
			panic(fmt.Errorf("kalodata-service: load accounts %q: %w", c.Kalodata.AccountsFile, err))
		}
		if len(accounts) == 0 {
			panic(fmt.Errorf("kalodata-service: accounts file %q has no usable accounts", c.Kalodata.AccountsFile))
		}
		if err := registry.SyncAccounts(context.Background(), accounts); err != nil {
			panic(fmt.Errorf("kalodata-service: sync accounts: %w", err))
		}
	} else {
		cookies := kalodata.ParseCookies(c.Kalodata.Cookies)
		if len(cookies) == 0 {
			panic("kalodata-service: set KALODATA_ACCOUNTS_FILE (kalodata_full_info.json) or KALODATA_COOKIES")
		}
		if err := registry.SyncCookies(context.Background(), cookies); err != nil {
			panic(fmt.Errorf("kalodata-service: sync cookies: %w", err))
		}
	}

	client := kalodata.NewHTTPClient(c.Kalodata.BaseURL, c.Kalodata.CDNBase, registry, c.Kalodata.Retries)
	client.SetLocale(c.Kalodata.Currency, c.Kalodata.Language)

	return &ServiceContext{
		Config: c,
		DB:     conn,
		Client: client,
	}
}
