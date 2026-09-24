// Code scaffolded by goctl. Safe to edit.
package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/amazon-service/amazon"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetAmazonJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetAmazonJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAmazonJobLogic {
	return &GetAmazonJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetAmazonJobLogic) GetAmazonJob(req *types.GetAmazonJobRequest) (resp *types.GetAmazonJobResponse, err error) {
	rpcResp, err := l.svcCtx.AmazonRpc.GetAmazonJob(l.ctx, &amazon.GetAmazonJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetAmazonJobResponse{
		Status:       rpcResp.Status,
		ProductCount: rpcResp.ProductCount,
		AiOutput:     rpcResp.AiOutput,
	}, nil
}
