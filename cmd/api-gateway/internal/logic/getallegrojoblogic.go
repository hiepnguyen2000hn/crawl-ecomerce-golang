package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/allegro"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetAllegroJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetAllegroJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAllegroJobLogic {
	return &GetAllegroJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetAllegroJobLogic) GetAllegroJob(req *types.GetAllegroJobRequest) (resp *types.GetAllegroJobResponse, err error) {
	rpcResp, err := l.svcCtx.AllegroRpc.GetAllegroJob(l.ctx, &allegro.GetAllegroJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetAllegroJobResponse{
		Status:       rpcResp.Status,
		ProductCount: rpcResp.ProductCount,
		AiOutput:     rpcResp.AiOutput,
	}, nil
}
