// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-service/reddit"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRedditJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRedditJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRedditJobLogic {
	return &GetRedditJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRedditJobLogic) GetRedditJob(req *types.GetRedditJobRequest) (resp *types.GetRedditJobResponse, err error) {
	rpcResp, err := l.svcCtx.RedditRpc.GetRedditJob(l.ctx, &reddit.GetRedditJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetRedditJobResponse{
		Status:       rpcResp.Status,
		PostCount:    rpcResp.PostCount,
		CommentCount: rpcResp.CommentCount,
		AiOutput:     rpcResp.AiOutput,
	}, nil
}
