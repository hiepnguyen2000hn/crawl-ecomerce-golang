package logic

import (
	"context"
	"encoding/json"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/linkedin-service/linkedin"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLinkedInPostsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetLinkedInPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLinkedInPostsLogic {
	return &GetLinkedInPostsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetLinkedInPostsLogic) GetLinkedInPosts(req *types.GetLinkedInPostsRequest) (resp *types.GetLinkedInPostsResponse, err error) {
	rpcResp, err := l.svcCtx.LinkedInRpc.GetLinkedInPosts(l.ctx, &linkedin.GetLinkedInPostsRequest{JobId: req.Id})
	if err != nil {
		return nil, err
	}

	out := &types.GetLinkedInPostsResponse{Posts: make([]types.LinkedInPostItem, 0, len(rpcResp.Posts))}
	for _, p := range rpcResp.Posts {
		raw := json.RawMessage(p.Raw)
		if len(raw) == 0 {
			raw = json.RawMessage("null")
		}
		out.Posts = append(out.Posts, types.LinkedInPostItem{
			PostId:        p.PostId,
			Url:           p.Url,
			Content:       p.Content,
			AuthorName:    p.AuthorName,
			AuthorUrl:     p.AuthorUrl,
			PostedAt:      p.PostedAt,
			Likes:         p.Likes,
			CommentsCount: p.CommentsCount,
			Shares:        p.Shares,
			Raw:           raw,
		})
	}
	return out, nil
}
