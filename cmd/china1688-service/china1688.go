package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/china1688-service/china1688"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/china1688-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/china1688-service/internal/server"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/china1688-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/china1688-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/china1688.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.Apify.ApiToken == "" || strings.Contains(c.Apify.ApiToken, "${") {
		panic("china1688-service: APIFY_API_TOKEN is not set")
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
		if err := consumer.Consume(context.Background(), "china1688.jobs", "china1688.jobs", w.HandleMessage); err != nil {
			fmt.Println("china1688-service consumer stopped:", err)
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		china1688.RegisterChina1688ServiceServer(grpcServer, server.NewChina1688ServiceServer(svcCtx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting china1688-service rpc server at %s...\n", c.ListenOn)
	serviceGroup := service.NewServiceGroup()
	serviceGroup.Add(s)
	serviceGroup.Start()
}
