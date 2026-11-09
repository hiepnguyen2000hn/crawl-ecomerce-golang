package svc

import (
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/crawler"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
)

type ServiceContext struct {
	Config  config.Config
	DB      *sql.DB
	Crawler crawler.Client
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}

	return &ServiceContext{
		Config: c,
		DB:     conn,
		Crawler: crawler.NewSubprocess(crawler.Config{
			Python:     c.Allegro.Python,
			Script:     c.Allegro.Script,
			ProfileDir: c.Allegro.ProfileDir,
			Proxy:      c.Allegro.Proxy,
			ProxyFile:  c.Allegro.ProxyFile,
			Xvfb:       c.Allegro.Xvfb,
		}),
	}
}
