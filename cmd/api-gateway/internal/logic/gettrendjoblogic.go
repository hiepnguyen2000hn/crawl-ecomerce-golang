// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/trend"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetTrendJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetTrendJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTrendJobLogic {
	return &GetTrendJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetTrendJobLogic) GetTrendJob(req *types.GetTrendJobRequest) (resp *types.GetTrendJobResponse, err error) {
	rpcResp, err := l.svcCtx.TrendRpc.GetTrendJob(l.ctx, &trend.GetTrendJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetTrendJobResponse{
		Status:   rpcResp.Status,
		Keyword:  rpcResp.Keyword,
		AiOutput: rpcResp.AiOutput,
	}, nil
}
