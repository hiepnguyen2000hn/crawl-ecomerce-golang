package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/china1688-service/china1688"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/china1688-service/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetChina1688JobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetChina1688JobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetChina1688JobLogic {
	return &GetChina1688JobLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetChina1688JobLogic) GetChina1688Job(in *china1688.GetChina1688JobRequest) (*china1688.GetChina1688JobResponse, error) {
	var status string
	err := l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT status FROM jobs WHERE id = $1`, in.JobId).Scan(&status)
	if err == sql.ErrNoRows {
		return &china1688.GetChina1688JobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	var productCount int32
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM china1688_raw WHERE job_id = $1`, in.JobId,
	).Scan(&productCount); err != nil {
		return nil, err
	}

	resp := &china1688.GetChina1688JobResponse{Status: status, ProductCount: productCount}

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
