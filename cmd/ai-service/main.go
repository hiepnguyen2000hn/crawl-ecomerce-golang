package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/ai-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/aiproviders"
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
}

var configFile = flag.String("f", "etc/ai.yaml", "config file")

func main() {
	flag.Parse()

	var c Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.OpenRouter.ApiKey == "" || strings.Contains(c.OpenRouter.ApiKey, "${") {
		panic("ai-service: OpenRouter.ApiKey is empty or unexpanded (set OPENROUTER_API_KEY env var)")
	}

	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}

	provider := aiproviders.NewOpenRouter(c.OpenRouter.ApiKey, c.OpenRouter.Model, c.OpenRouter.BaseURL, http.DefaultClient, c.OpenRouter.FallbackModels...)

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	fbadsConsumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	amazonConsumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	w := &worker.Consumer{DB: conn, Provider: provider}
	fmt.Println("Starting ai-service consumer...")

	go func() {
		if err := consumer.Consume(context.Background(), "ai.trend.completed", "crawl.completed.trend", w.HandleMessage); err != nil {
			fmt.Fprintln(os.Stderr, "ai-service trend consumer stopped:", err)
			os.Exit(1)
		}
	}()

	go func() {
		if err := fbadsConsumer.Consume(context.Background(), "ai.fbads.completed", "crawl.completed.fbads", w.HandleMessage); err != nil {
			fmt.Fprintln(os.Stderr, "ai-service fbads consumer stopped:", err)
			os.Exit(1)
		}
	}()

	go func() {
		if err := amazonConsumer.Consume(context.Background(), "ai.amazon.completed", "crawl.completed.amazon", w.HandleMessage); err != nil {
			fmt.Fprintln(os.Stderr, "ai-service amazon consumer stopped:", err)
			os.Exit(1)
		}
	}()

	select {}
}
