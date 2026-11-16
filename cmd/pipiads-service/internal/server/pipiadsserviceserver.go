package server

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/internal/logic"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/pipiads"
)

type PipiadsServiceServer struct {
	svcCtx *svc.ServiceContext
	pipiads.UnimplementedPipiadsServiceServer
}

func NewPipiadsServiceServer(svcCtx *svc.ServiceContext) *PipiadsServiceServer {
	return &PipiadsServiceServer{
		svcCtx: svcCtx,
	}
}

func (s *PipiadsServiceServer) GetPipiadsJob(ctx context.Context, in *pipiads.GetPipiadsJobRequest) (*pipiads.GetPipiadsJobResponse, error) {
	l := logic.NewGetPipiadsJobLogic(ctx, s.svcCtx)
	return l.GetPipiadsJob(in)
}
