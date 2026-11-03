package server

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/allegro"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/logic"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/svc"
)

type AllegroServiceServer struct {
	svcCtx *svc.ServiceContext
	allegro.UnimplementedAllegroServiceServer
}

func NewAllegroServiceServer(svcCtx *svc.ServiceContext) *AllegroServiceServer {
	return &AllegroServiceServer{
		svcCtx: svcCtx,
	}
}

func (s *AllegroServiceServer) GetAllegroJob(ctx context.Context, in *allegro.GetAllegroJobRequest) (*allegro.GetAllegroJobResponse, error) {
	l := logic.NewGetAllegroJobLogic(ctx, s.svcCtx)
	return l.GetAllegroJob(in)
}
