package svc

import (
	"context"
	"database/sql"
	"log"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/pipiads"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

type ServiceContext struct {
	Config    config.Config
	DB        *sql.DB
	Client    pipiads.Client
	Registry  *pipiads.AccountRegistry
	Publisher *rabbitmq.Publisher
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		log.Fatalf("pipiads-service: failed to connect to postgres: %v", err)
	}

	client := pipiads.NewHTTPClient(c.Pipiads.BaseURL)
	registry := pipiads.NewAccountRegistry(conn, client)

	// Sync accounts if file exists
	if c.Pipiads.AccountsFile != "" {
		if err := registry.SyncAccounts(context.Background(), c.Pipiads.AccountsFile); err != nil {
			log.Printf("pipiads-service: warning syncing accounts file %s: %v", c.Pipiads.AccountsFile, err)
		}
	}

	pub, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		log.Printf("pipiads-service: warning init rabbitmq publisher: %v", err)
	}

	return &ServiceContext{
		Config:    c,
		DB:        conn,
		Client:    client,
		Registry:  registry,
		Publisher: pub,
	}
}
