package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/kalodata-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/kalodata-service/internal/server"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/kalodata-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/kalodata-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/kalodata-service/kalodata"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/kalodata.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

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

	w := &worker.Consumer{DB: svcCtx.DB, Client: svcCtx.Client, Searcher: svcCtx.Searcher, Publisher: publisher}
	go func() {
		if err := consumer.Consume(context.Background(), "kalodata.jobs", "kalodata.jobs", w.HandleMessage); err != nil {
			fmt.Println("kalodata-service consumer stopped:", err)
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		kalodata.RegisterKalodataServiceServer(grpcServer, server.NewKalodataServiceServer(svcCtx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting kalodata-service rpc server at %s...\n", c.ListenOn)
	serviceGroup := service.NewServiceGroup()
	serviceGroup.Add(s)
	serviceGroup.Start()
}
