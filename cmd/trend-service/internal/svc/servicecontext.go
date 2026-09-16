package svc

import (
	"database/sql"
	"net/http"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"
)

type ServiceContext struct {
	Config  config.Config
	DB      *sql.DB
	SerpApi serpapi.Client
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	return &ServiceContext{
		Config:  c,
		DB:      conn,
		SerpApi: serpapi.NewHTTPClient(c.SerpApi.ApiKey, c.SerpApi.BaseURL, http.DefaultClient),
	}
}
