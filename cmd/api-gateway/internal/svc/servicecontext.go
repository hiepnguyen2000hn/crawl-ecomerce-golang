// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package svc

import (
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/amazon-service/amazon"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/config"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/fbads"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/trend"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/rabbitmq"

	"google.golang.org/grpc"
)

type ServiceContext struct {
	Config    config.Config
	DB        *sql.DB
	Publisher *rabbitmq.Publisher
	TrendRpc  trend.TrendServiceClient
	FbAdsRpc  fbads.FbAdsServiceClient
	AmazonRpc amazon.AmazonServiceClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := db.Connect(c.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	publisher, err := rabbitmq.NewPublisher(rabbitmq.Config{URL: c.RabbitMQ.URL, Exchange: c.RabbitMQ.Exchange})
	if err != nil {
		panic(err)
	}
	grpcConn, err := grpc.NewClient(c.TrendRpc.Target, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	fbAdsGrpcConn, err := grpc.NewClient(c.FbAdsRpc.Target, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	amazonGrpcConn, err := grpc.NewClient(c.AmazonRpc.Target, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	return &ServiceContext{
		Config:    c,
		DB:        conn,
		Publisher: publisher,
		TrendRpc:  trend.NewTrendServiceClient(grpcConn),
		FbAdsRpc:  fbads.NewFbAdsServiceClient(fbAdsGrpcConn),
		AmazonRpc: amazon.NewAmazonServiceClient(amazonGrpcConn),
	}
}
