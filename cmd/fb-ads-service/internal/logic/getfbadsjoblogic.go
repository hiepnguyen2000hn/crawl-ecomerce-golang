package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/fbads"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/fb-ads-service/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFbAdsJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetFbAdsJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFbAdsJobLogic {
	return &GetFbAdsJobLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetFbAdsJobLogic) GetFbAdsJob(in *fbads.GetFbAdsJobRequest) (*fbads.GetFbAdsJobResponse, error) {
	var status string
	err := l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT status FROM jobs WHERE id = $1`, in.JobId).Scan(&status)
	if err == sql.ErrNoRows {
		return &fbads.GetFbAdsJobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	var adCount int32
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM fbads_raw WHERE job_id = $1`, in.JobId,
	).Scan(&adCount); err != nil {
		return nil, err
	}

	resp := &fbads.GetFbAdsJobResponse{Status: status, AdCount: adCount}

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
