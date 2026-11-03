package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/allegro"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/server"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "cmd/allegro-service/etc/allegro.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	publisher, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}
	defer publisher.Close()

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{
		URL:         c.RabbitMQ.URL,
		Exchange:    c.RabbitMQ.Exchange,
		Concurrency: c.RabbitMQ.Concurrency,
	})
	if err != nil {
		panic(err)
	}
	defer consumer.Close()

	w := &worker.Consumer{
		DB:        svcCtx.DB,
		Crawler:   svcCtx.Crawler,
		Publisher: publisher,
		Timeout:   time.Duration(c.Allegro.TimeoutSeconds) * time.Second,
	}
	go func() {
		if err := consumer.Consume(context.Background(), "allegro.jobs", "allegro.jobs", w.HandleMessage); err != nil {
			fmt.Println("allegro-service consumer stopped:", err)
		}
	}()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		allegro.RegisterAllegroServiceServer(grpcServer, server.NewAllegroServiceServer(svcCtx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting allegro-service rpc server at %s...\n", c.ListenOn)
	serviceGroup := service.NewServiceGroup()
	serviceGroup.Add(s)
	serviceGroup.Start()
}
