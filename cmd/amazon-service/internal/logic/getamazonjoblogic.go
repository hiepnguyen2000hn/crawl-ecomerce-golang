package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/amazon-service/amazon"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/amazon-service/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetAmazonJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetAmazonJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAmazonJobLogic {
	return &GetAmazonJobLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetAmazonJobLogic) GetAmazonJob(in *amazon.GetAmazonJobRequest) (*amazon.GetAmazonJobResponse, error) {
	var status string
	err := l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT status FROM jobs WHERE id = $1`, in.JobId).Scan(&status)
	if err == sql.ErrNoRows {
		return &amazon.GetAmazonJobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	var productCount int32
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM amazon_raw WHERE job_id = $1`, in.JobId,
	).Scan(&productCount); err != nil {
		return nil, err
	}

	resp := &amazon.GetAmazonJobResponse{Status: status, ProductCount: productCount}

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
