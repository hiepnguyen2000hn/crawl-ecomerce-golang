// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-api-service/redditapi"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRedditApiJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRedditApiJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRedditApiJobLogic {
	return &GetRedditApiJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRedditApiJobLogic) GetRedditApiJob(req *types.GetRedditApiJobRequest) (resp *types.GetRedditApiJobResponse, err error) {
	rpcResp, err := l.svcCtx.RedditApiRpc.GetRedditApiJob(l.ctx, &redditapi.GetRedditApiJobRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}
	return &types.GetRedditApiJobResponse{
		Status:       rpcResp.Status,
		PostCount:    rpcResp.PostCount,
		CommentCount: rpcResp.CommentCount,
		AiOutput:     rpcResp.AiOutput,
	}, nil
}
