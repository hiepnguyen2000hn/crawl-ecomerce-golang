package logic

import (
	"context"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/linkedin-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/linkedin-service/linkedin"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLinkedInPostsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetLinkedInPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLinkedInPostsLogic {
	return &GetLinkedInPostsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetLinkedInPostsLogic) GetLinkedInPosts(in *linkedin.GetLinkedInPostsRequest) (*linkedin.GetLinkedInPostsResponse, error) {
	rows, err := l.svcCtx.DB.QueryContext(l.ctx,
		`SELECT post_id, COALESCE(url,''), COALESCE(content,''), COALESCE(author_name,''),
		        COALESCE(author_url,''), COALESCE(to_char(posted_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
		        COALESCE(likes,0), COALESCE(comments_count,0), COALESCE(shares,0), raw::text
		 FROM linkedin_posts_raw WHERE job_id = $1 ORDER BY posted_at DESC NULLS LAST, fetched_at DESC`,
		in.JobId,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resp := &linkedin.GetLinkedInPostsResponse{}
	for rows.Next() {
		var p linkedin.LinkedInPost
		if err := rows.Scan(
			&p.PostId, &p.Url, &p.Content, &p.AuthorName, &p.AuthorUrl, &p.PostedAt,
			&p.Likes, &p.CommentsCount, &p.Shares, &p.Raw,
		); err != nil {
			return nil, err
		}
		resp.Posts = append(resp.Posts, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}
