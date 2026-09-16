package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/server"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/trend"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/trend.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	svcCtx := svc.NewServiceContext(c)

	publisher, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}

	w := &worker.Consumer{DB: svcCtx.DB, SerpApi: svcCtx.SerpApi, Publisher: publisher}
	go func() {
		if err := consumer.Consume(context.Background(), "trend.jobs", "trend.jobs", w.HandleMessage); err != nil {
			fmt.Println("trend-service consumer stopped:", err)
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		trend.RegisterTrendServiceServer(grpcServer, server.NewTrendServiceServer(svcCtx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting trend-service rpc server at %s...\n", c.ListenOn)
	serviceGroup := service.NewServiceGroup()
	serviceGroup.Add(s)
	serviceGroup.Start()
}
