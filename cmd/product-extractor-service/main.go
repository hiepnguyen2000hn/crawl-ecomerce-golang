package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/product-extractor-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/crawl4ai"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
)

type Config struct {
	Postgres struct {
		DSN string
	}
	RabbitMQ struct {
		URL      string
		Exchange string
	}
	OpenRouter struct {
		ApiKey         string
		Model          string
		FallbackModels []string `json:",optional"`
		BaseURL        string
	}
	Crawl4ai struct {
		BaseURL string
	}
	// Số URL bóc song song trong 1 job và trần thời gian mỗi URL (giây). 0 → mặc định của worker.
	Concurrency   int `json:",optional"`
	URLTimeoutSec int `json:",optional"`
}

var configFile = flag.String("f", "etc/extractor.yaml", "config file")

func main() {
	flag.Parse()

	var c Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.OpenRouter.ApiKey == "" || strings.Contains(c.OpenRouter.ApiKey, "${") {
		panic("product-extractor-service: OpenRouter.ApiKey is empty or unexpanded (set OPENROUTER_API_KEY env var)")
	}

	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}

	// Timeout riêng cho LLM: http.DefaultClient không có timeout, model free có lúc treo không trả.
	provider := aiproviders.NewOpenRouter(c.OpenRouter.ApiKey, c.OpenRouter.Model, c.OpenRouter.BaseURL, &http.Client{Timeout: 60 * time.Second}, c.OpenRouter.FallbackModels...)
	crawler := crawl4ai.NewHTTPClient(c.Crawl4ai.BaseURL, &http.Client{Timeout: 2 * time.Minute}) // crawl4ai pages can be slow

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	w := &worker.Consumer{
		DB: conn, Crawler: crawler, Provider: provider,
		Concurrency: c.Concurrency, URLTimeout: time.Duration(c.URLTimeoutSec) * time.Second,
	}
	fmt.Println("Starting product-extractor-service consumer...")
	if err := consumer.Consume(context.Background(), "product-extractor.fbads.completed", "crawl.completed.fbads", w.HandleMessage); err != nil {
		fmt.Fprintln(os.Stderr, "product-extractor-service consumer stopped:", err)
		os.Exit(1)
	}
}
