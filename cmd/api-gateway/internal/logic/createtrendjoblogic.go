// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package logic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateTrendJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateTrendJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateTrendJobLogic {
	return &CreateTrendJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateTrendJobLogic) CreateTrendJob(req *types.CreateTrendJobRequest) (resp *types.CreateTrendJobResponse, err error) {
	if req.Keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}

	params, err := json.Marshal(map[string]string{"keyword": req.Keyword, "geo": req.Geo})
	if err != nil {
		return nil, err
	}

	var jobID string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('trend', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	msg, err := json.Marshal(map[string]string{"job_id": jobID, "keyword": req.Keyword, "geo": req.Geo})
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, "trend.jobs", msg); err != nil {
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &types.CreateTrendJobResponse{JobId: jobID}, nil
}
