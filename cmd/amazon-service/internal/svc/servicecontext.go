package svc

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/amazon-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
)

type ServiceContext struct {
	Config config.Config
	DB     *sql.DB
	Apify  apify.ProductClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	return &ServiceContext{
		Config: c,
		DB:     conn,
		Apify:  apify.NewAmazonHTTPClient(c.Apify.ApiToken, c.Apify.ActorID, c.Apify.BaseURL, &http.Client{Timeout: 5 * time.Minute}),
	}
}
