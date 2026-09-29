// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/china1688-service/china1688"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetChina1688JobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetChina1688JobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetChina1688JobLogic {
	return &GetChina1688JobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetChina1688JobLogic) GetChina1688Job(req *types.GetChina1688JobRequest) (resp *types.GetChina1688JobResponse, err error) {
	rpcResp, err := l.svcCtx.China1688Rpc.GetChina1688Job(l.ctx, &china1688.GetChina1688JobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetChina1688JobResponse{
		Status:       rpcResp.Status,
		ProductCount: rpcResp.ProductCount,
		AiOutput:     rpcResp.AiOutput,
	}, nil
}
