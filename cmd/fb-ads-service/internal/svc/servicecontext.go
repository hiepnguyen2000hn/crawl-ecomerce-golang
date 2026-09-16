package svc

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
)

type ServiceContext struct {
	Config config.Config
	DB     *sql.DB
	Apify  apify.Client
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	return &ServiceContext{
		Config: c,
		DB:     conn,
		Apify:  apify.NewHTTPClient(c.Apify.ApiToken, c.Apify.ActorID, c.Apify.BaseURL, &http.Client{Timeout: 5 * time.Minute}),
	}
}
