package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/kalodata-service/kalodata"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetKalodataJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetKalodataJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetKalodataJobLogic {
	return &GetKalodataJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetKalodataJobLogic) GetKalodataJob(req *types.GetKalodataJobRequest) (resp *types.GetKalodataJobResponse, err error) {
	rpcResp, err := l.svcCtx.KalodataRpc.GetKalodataJob(l.ctx, &kalodata.GetKalodataJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetKalodataJobResponse{
		Status:       rpcResp.Status,
		ProductCount: rpcResp.ProductCount,
		VideoCount:   rpcResp.VideoCount,
		AiOutput:     rpcResp.AiOutput,
	}, nil
}
