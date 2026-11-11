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

	cookies := kalodata.ParseCookies(c.Kalodata.Cookies)
	if len(cookies) == 0 {
		panic("kalodata-service: KALODATA_COOKIES is empty (one account's Cookie header per line)")
	}

	registry := kalodata.NewCookieRegistry(conn, ttl, c.Kalodata.DailySearchQuota, c.Kalodata.DailyDetailQuota)
	if err := registry.SyncCookies(context.Background(), cookies); err != nil {
		panic(fmt.Errorf("kalodata-service: sync cookies: %w", err))
	}

	return &ServiceContext{
		Config: c,
		DB:     conn,
		Client: kalodata.NewHTTPClient(c.Kalodata.BaseURL, c.Kalodata.CDNBase, registry, c.Kalodata.Retries),
	}
}
