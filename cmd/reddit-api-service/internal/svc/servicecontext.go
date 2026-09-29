package svc

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-api-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/redditapi"
)

type ServiceContext struct {
	Config config.Config
	DB     *sql.DB
	Reddit redditapi.RedditClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	return &ServiceContext{
		Config: c,
		DB:     conn,
		Reddit: redditapi.NewRedditHTTPClient(c.Reddit.ClientID, c.Reddit.ClientSecret, c.Reddit.UserAgent, c.Reddit.AuthURL, c.Reddit.ApiURL, &http.Client{Timeout: 2 * time.Minute}),
	}
}
