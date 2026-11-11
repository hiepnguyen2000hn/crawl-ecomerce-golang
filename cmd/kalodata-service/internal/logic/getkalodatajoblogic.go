package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/kalodata-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/kalodata-service/kalodata"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetKalodataJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetKalodataJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetKalodataJobLogic {
	return &GetKalodataJobLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetKalodataJobLogic) GetKalodataJob(in *kalodata.GetKalodataJobRequest) (*kalodata.GetKalodataJobResponse, error) {
	var status string
	err := l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT status FROM jobs WHERE id = $1`, in.JobId).Scan(&status)
	if err == sql.ErrNoRows {
		return &kalodata.GetKalodataJobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	var productCount, videoCount int32
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM kalodata_products WHERE job_id = $1`, in.JobId,
	).Scan(&productCount); err != nil {
		return nil, err
	}
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM kalodata_videos WHERE job_id = $1`, in.JobId,
	).Scan(&videoCount); err != nil {
		return nil, err
	}

	resp := &kalodata.GetKalodataJobResponse{Status: status, ProductCount: productCount, VideoCount: videoCount}

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
