package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/allegro"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/allegro-service/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetAllegroJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetAllegroJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAllegroJobLogic {
	return &GetAllegroJobLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetAllegroJobLogic) GetAllegroJob(in *allegro.GetAllegroJobRequest) (*allegro.GetAllegroJobResponse, error) {
	var status string
	err := l.svcCtx.DB.QueryRowContext(l.ctx, `SELECT status FROM jobs WHERE id = $1`, in.JobId).Scan(&status)
	if err == sql.ErrNoRows {
		return &allegro.GetAllegroJobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	var productCount int32
	if err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT COUNT(*) FROM allegro_raw WHERE job_id = $1`, in.JobId,
	).Scan(&productCount); err != nil {
		return nil, err
	}

	resp := &allegro.GetAllegroJobResponse{Status: status, ProductCount: productCount}

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
