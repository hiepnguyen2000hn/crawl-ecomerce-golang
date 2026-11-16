package logic

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/pipiads"
)

type GetPipiadsJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPipiadsJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPipiadsJobLogic {
	return &GetPipiadsJobLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetPipiadsJobLogic) GetPipiadsJob(in *pipiads.GetPipiadsJobRequest) (*pipiads.GetPipiadsJobResponse, error) {
	var status string
	err := l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT status FROM jobs WHERE id = $1`, in.JobId).Scan(&status)
	if err != nil {
		return nil, err
	}

	var count int32
	_ = l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT COUNT(*) FROM pipiads_ads WHERE job_id = $1`, in.JobId).Scan(&count)

	return &pipiads.GetPipiadsJobResponse{
		Status:   status,
		AdsCount: count,
	}, nil
}
