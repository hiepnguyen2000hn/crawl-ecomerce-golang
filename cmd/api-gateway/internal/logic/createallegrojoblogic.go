package logic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateAllegroJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateAllegroJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAllegroJobLogic {
	return &CreateAllegroJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateAllegroJobLogic) CreateAllegroJob(req *types.CreateAllegroJobRequest) (resp *types.CreateAllegroJobResponse, err error) {
	if req.Keyword == "" && req.Url == "" && len(req.Urls) == 0 {
		return nil, fmt.Errorf("either keyword, url, or urls is required")
	}

	params, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var jobID string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('allegro', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	msg, err := json.Marshal(map[string]interface{}{
		"job_id":    jobID,
		"keyword":   req.Keyword,
		"url":       req.Url,
		"urls":      req.Urls,
		"pages":     req.Pages,
		"max_items": req.MaxItems,
	})
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, "allegro.jobs", msg); err != nil {
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &types.CreateAllegroJobResponse{JobId: jobID}, nil
}
