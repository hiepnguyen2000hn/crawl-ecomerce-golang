package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/linkedin-service/linkedin"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLinkedInJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetLinkedInJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLinkedInJobLogic {
	return &GetLinkedInJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetLinkedInJobLogic) GetLinkedInJob(req *types.GetLinkedInJobRequest) (resp *types.GetLinkedInJobResponse, err error) {
	rpcResp, err := l.svcCtx.LinkedInRpc.GetLinkedInJob(l.ctx, &linkedin.GetLinkedInJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetLinkedInJobResponse{
		Status:    rpcResp.Status,
		PostCount: rpcResp.PostCount,
		AiOutput:  rpcResp.AiOutput,
	}, nil
}
