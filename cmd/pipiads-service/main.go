package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/internal/server"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/internal/worker"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/pipiads"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"
)

var configFile = flag.String("f", "etc/pipiads.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)

	consumer, err := rabbitmq.NewConsumer(rabbitmq.Config{
		URL:         c.RabbitMQ.URL,
		Exchange:    c.RabbitMQ.Exchange,
		Concurrency: c.RabbitMQ.Concurrency,
	})
	if err != nil {
		log.Printf("pipiads-service: warning rabbitmq init error: %v", err)
	} else {
		w := &worker.Consumer{
			DB:        ctx.DB,
			Client:    ctx.Client,
			Registry:  ctx.Registry,
			Publisher: ctx.Publisher,
		}
		go func() {
			if err := consumer.Consume(context.Background(), "pipiads.jobs", "crawl.pipiads", w.HandleMessage); err != nil {
				log.Printf("pipiads-service: rabbitmq consumer stopped: %v", err)
			}
		}()
	}

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pipiads.RegisterPipiadsServiceServer(grpcServer, server.NewPipiadsServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting pipiads rpc server at %s...\n", c.ListenOn)
	s.Start()
}
