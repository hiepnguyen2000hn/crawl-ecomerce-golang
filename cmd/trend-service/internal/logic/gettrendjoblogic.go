package logic

import (
	"context"
	"database/sql"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/trend-service/trend"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetTrendJobLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetTrendJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTrendJobLogic {
	return &GetTrendJobLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetTrendJobLogic) GetTrendJob(in *trend.GetTrendJobRequest) (*trend.GetTrendJobResponse, error) {
	var status, keyword string
	err := l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT status, params->>'keyword' FROM jobs WHERE id = $1`, in.JobId,
	).Scan(&status, &keyword)
	if err == sql.ErrNoRows {
		return &trend.GetTrendJobResponse{Status: "not_found"}, nil
	}
	if err != nil {
		return nil, err
	}

	resp := &trend.GetTrendJobResponse{Status: status, Keyword: keyword}

	// Points are parsed from trend_data JSONB in the api-gateway; the RPC
	// response carries raw status/keyword/ai_output here and leaves point
	// parsing to the caller by returning an empty Points slice when absent.
	var trendData []byte
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`SELECT trend_data FROM trend_raw WHERE job_id = $1 ORDER BY fetched_at DESC LIMIT 1`, in.JobId,
	).Scan(&trendData)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

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
