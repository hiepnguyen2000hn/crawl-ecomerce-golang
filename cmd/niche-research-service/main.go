package main

import (
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/niche-research-service/internal/handler"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/niche-research-service/internal/service"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/serpapi"

	"github.com/zeromicro/go-zero/core/conf"
)

type Config struct {
	ListenOn string
	Postgres struct {
		DSN string
	}
	OpenRouter struct {
		ApiKey         string
		Model          string
		FallbackModels []string `json:",optional"`
		BaseURL        string
	}
	SerpApi struct {
		ApiKey  string
		BaseURL string
	}
}

var configFile = flag.String("f", "etc/niche.yaml", "config file")

func main() {
	flag.Parse()

	var c Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.OpenRouter.ApiKey == "" || strings.Contains(c.OpenRouter.ApiKey, "${") {
		panic("niche-research-service: OpenRouter.ApiKey is empty or unexpanded (set OPENROUTER_API_KEY env var)")
	}
	if c.SerpApi.ApiKey == "" || strings.Contains(c.SerpApi.ApiKey, "${") {
		panic("niche-research-service: SerpApi.ApiKey is empty or unexpanded (set SERPAPI_API_KEY env var)")
	}

	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}

	svc := &service.Service{
		DB:     conn,
		AI:     aiproviders.NewOpenRouter(c.OpenRouter.ApiKey, c.OpenRouter.Model, c.OpenRouter.BaseURL, http.DefaultClient, c.OpenRouter.FallbackModels...),
		Trends: serpapi.NewHTTPClient(c.SerpApi.ApiKey, c.SerpApi.BaseURL, http.DefaultClient),
	}
	h := &handler.Handler{Service: svc}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/niche/sessions", h.Sessions)

	fmt.Println("Starting niche-research-service at", c.ListenOn)
	if err := http.ListenAndServe(c.ListenOn, handler.WithCORS(mux)); err != nil {
		panic(err)
	}
}
