package logic

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/pipiads-service/pipiads"
)

type GetPipiadsJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetPipiadsJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPipiadsJobLogic {
	return &GetPipiadsJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPipiadsJobLogic) GetPipiadsJob(req *types.GetPipiadsJobRequest) (resp *types.GetPipiadsJobResponse, err error) {
	rpcResp, err := l.svcCtx.PipiadsRpc.GetPipiadsJob(l.ctx, &pipiads.GetPipiadsJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}

	var aiOutput string
	_ = l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT output FROM ai_results WHERE job_id = $1 ORDER BY created_at DESC LIMIT 1`, req.Id,
	).Scan(&aiOutput)

	return &types.GetPipiadsJobResponse{
		Status:   rpcResp.Status,
		AdsCount: rpcResp.AdsCount,
		AiOutput: aiOutput,
	}, nil
}
