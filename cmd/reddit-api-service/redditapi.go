package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-api-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-api-service/internal/server"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-api-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-api-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-api-service/redditapi"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/redditapi.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.Reddit.ClientID == "" || strings.Contains(c.Reddit.ClientID, "${") {
		panic("reddit-api-service: REDDIT_CLIENT_ID is not set")
	}
	if c.Reddit.ClientSecret == "" || strings.Contains(c.Reddit.ClientSecret, "${") {
		panic("reddit-api-service: REDDIT_CLIENT_SECRET is not set")
	}

	svcCtx := svc.NewServiceContext(c)

	publisher, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	w := &worker.Consumer{DB: svcCtx.DB, Reddit: svcCtx.Reddit, Publisher: publisher}
	go func() {
		if err := consumer.Consume(context.Background(), "reddit-api.jobs", "reddit-api.jobs", w.HandleMessage); err != nil {
			fmt.Println("reddit-api-service consumer stopped:", err)
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		redditapi.RegisterRedditApiServiceServer(grpcServer, server.NewRedditApiServiceServer(svcCtx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting reddit-api-service rpc server at %s...\n", c.ListenOn)
	serviceGroup := service.NewServiceGroup()
	serviceGroup.Add(s)
	serviceGroup.Start()
}
