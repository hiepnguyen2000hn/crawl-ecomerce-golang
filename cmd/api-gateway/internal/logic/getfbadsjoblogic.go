// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/fbads"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFbAdsJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetFbAdsJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFbAdsJobLogic {
	return &GetFbAdsJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetFbAdsJobLogic) GetFbAdsJob(req *types.GetFbAdsJobRequest) (resp *types.GetFbAdsJobResponse, err error) {
	rpcResp, err := l.svcCtx.FbAdsRpc.GetFbAdsJob(l.ctx, &fbads.GetFbAdsJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetFbAdsJobResponse{
		Status:   rpcResp.Status,
		AdCount:  rpcResp.AdCount,
		AiOutput: rpcResp.AiOutput,
	}, nil
}
