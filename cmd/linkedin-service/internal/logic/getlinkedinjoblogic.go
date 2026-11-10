package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/linkedin-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/linkedin-service/linkedin"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLinkedInJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetLinkedInJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLinkedInJobLogic {
	return &GetLinkedInJobLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetLinkedInJobLogic) GetLinkedInJob(in *linkedin.GetLinkedInJobRequest) (*linkedin.GetLinkedInJobResponse, error) {
	var status string
	err := l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT status FROM jobs WHERE id = $1`, in.JobId).Scan(&status)
	if err == sql.ErrNoRows {
		return &linkedin.GetLinkedInJobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	var postCount int32
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM linkedin_posts_raw WHERE job_id = $1`, in.JobId,
	).Scan(&postCount); err != nil {
		return nil, err
	}

	resp := &linkedin.GetLinkedInJobResponse{Status: status, PostCount: postCount}

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
