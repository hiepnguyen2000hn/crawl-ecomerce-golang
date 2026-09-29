package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-service/internal/server"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-service/reddit"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/reddit.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.Apify.ApiToken == "" || strings.Contains(c.Apify.ApiToken, "${") {
		panic("reddit-service: APIFY_API_TOKEN is not set")
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

	w := &worker.Consumer{DB: svcCtx.DB, Apify: svcCtx.Apify, Publisher: publisher}
	go func() {
		if err := consumer.Consume(context.Background(), "reddit.jobs", "reddit.jobs", w.HandleMessage); err != nil {
			fmt.Println("reddit-service consumer stopped:", err)
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		reddit.RegisterRedditServiceServer(grpcServer, server.NewRedditServiceServer(svcCtx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting reddit-service rpc server at %s...\n", c.ListenOn)
	serviceGroup := service.NewServiceGroup()
	serviceGroup.Add(s)
	serviceGroup.Start()
}
