package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/fbads"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/media"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/server"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/apify"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/fbads.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	// ParseTokens already discards blanks and the unexpanded "${...}"
	// placeholder go-zero leaves behind when the env var is missing.
	if len(apify.ParseTokens(c.Apify.ApiToken)) == 0 {
		panic("fb-ads-service: APIFY_API_TOKEN is not set (one token, or several separated by commas)")
	}

	svcCtx := svc.NewServiceContext(c)

	publisher, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{
		URL:         c.RabbitMQ.URL,
		Exchange:    c.RabbitMQ.Exchange,
		Concurrency: c.RabbitMQ.Concurrency,
	})
	if err != nil {
		panic(err)
	}

	mediaStore := &media.Store{DB: svcCtx.DB}
	w := &worker.Consumer{DB: svcCtx.DB, Apify: svcCtx.Apify, Publisher: publisher, Media: mediaStore}
	go func() {
		if err := consumer.Consume(context.Background(), "fbads.jobs", "fbads.jobs", w.HandleMessage); err != nil {
			fmt.Println("fb-ads-service consumer stopped:", err)
		}
	}()

	// fbcdn image URLs expire a few days after the crawl, so download them
	// as soon as the crawl completes. Own queue on the existing event, next
	// to ai-service's and product-extractor's.
	if err := os.MkdirAll(c.Media.Dir, 0o755); err != nil {
		panic(fmt.Errorf("fb-ads-service: media dir %s: %w", c.Media.Dir, err))
	}
	mediaConsumer, err := rabbitmq.NewConsumer(rabbitmq.Config{
		URL:         c.RabbitMQ.URL,
		Exchange:    c.RabbitMQ.Exchange,
		Concurrency: 1,
	})
	if err != nil {
		panic(err)
	}
	mediaWorker := &media.Worker{
		Repo: mediaStore,
		Fetcher: &media.Downloader{
			HTTP:     &http.Client{Timeout: 30 * time.Second},
			Dir:      c.Media.Dir,
			MaxBytes: c.Media.MaxBytes,
		},
		Concurrency: c.Media.Concurrency,
		Retries:     3,
	}
	go func() {
		if err := mediaConsumer.Consume(context.Background(), "fbads.media.completed", "crawl.completed.fbads", mediaWorker.HandleMessage); err != nil {
			fmt.Println("fb-ads-service media consumer stopped:", err)
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		fbads.RegisterFbAdsServiceServer(grpcServer, server.NewFbAdsServiceServer(svcCtx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting fb-ads-service rpc server at %s...\n", c.ListenOn)
	serviceGroup := service.NewServiceGroup()
	serviceGroup.Add(s)
	serviceGroup.Start()
}
