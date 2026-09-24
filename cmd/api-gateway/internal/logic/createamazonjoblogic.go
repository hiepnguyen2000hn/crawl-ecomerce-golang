// Code scaffolded by goctl. Safe to edit.
package logic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateAmazonJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateAmazonJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAmazonJobLogic {
	return &CreateAmazonJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateAmazonJobLogic) CreateAmazonJob(req *types.CreateAmazonJobRequest) (resp *types.CreateAmazonJobResponse, err error) {
	if req.Keyword == "" && len(req.Urls) == 0 {
		return nil, fmt.Errorf("either keyword or urls is required")
	}
	if req.MaxItems > 200 {
		return nil, fmt.Errorf("max_items must be 200 or less")
	}

	params, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var jobID string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('amazon', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	msg, err := json.Marshal(map[string]interface{}{
		"job_id":    jobID,
		"keyword":   req.Keyword,
		"urls":      req.Urls,
		"country":   req.Country,
		"max_items": req.MaxItems,
	})
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, "amazon.jobs", msg); err != nil {
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &types.CreateAmazonJobResponse{JobId: jobID}, nil
}
