package svc

import (
	"context"
	"database/sql"
	"fmt"
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

	// Concurrency accounting lives in Postgres because every Apify-calling
	// service is its own process sharing one token pool: a per-process
	// semaphore would let four services each run the full per-account limit.
	slots := apify.NewSlotStore(conn, apify.ParseTokens(c.Apify.ApiToken), "amazon-service", 6*time.Minute)
	if err := slots.SyncTokens(context.Background(), c.Apify.MaxConcurrencyPerToken); err != nil {
		panic(fmt.Errorf("amazon-service: register apify tokens: %w", err))
	}

	return &ServiceContext{
		Config: c,
		DB:     conn,
		Apify:  apify.NewAmazonHTTPClient(c.Apify.ActorID, apify.NewRunner(c.Apify.BaseURL, slots, 5*time.Minute)),
	}
}
