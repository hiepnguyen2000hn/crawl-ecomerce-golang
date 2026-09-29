package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-api-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/reddit-api-service/redditapi"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRedditApiJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetRedditApiJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRedditApiJobLogic {
	return &GetRedditApiJobLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetRedditApiJobLogic) GetRedditApiJob(in *redditapi.GetRedditApiJobRequest) (*redditapi.GetRedditApiJobResponse, error) {
	var status string
	err := l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT status FROM jobs WHERE id = $1`, in.JobId).Scan(&status)
	if err == sql.ErrNoRows {
		return &redditapi.GetRedditApiJobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	var postCount int32
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM reddit_posts_raw WHERE job_id = $1`, in.JobId,
	).Scan(&postCount); err != nil {
		return nil, err
	}

	var commentCount int32
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM reddit_comments_raw WHERE job_id = $1`, in.JobId,
	).Scan(&commentCount); err != nil {
		return nil, err
	}

	resp := &redditapi.GetRedditApiJobResponse{Status: status, PostCount: postCount, CommentCount: commentCount}

	var aiOutput string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT output->>'content' FROM ai_results WHERE job_id = $1 ORDER BY created_at DESC LIMIT 1`, in.JobId,
	).Scan(&aiOutput)
	if err == nil {
		resp.AiOutput = aiOutput
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	return resp, nil
}
