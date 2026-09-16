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

type CreateFbAdsJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateFbAdsJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateFbAdsJobLogic {
	return &CreateFbAdsJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateFbAdsJobLogic) CreateFbAdsJob(req *types.CreateFbAdsJobRequest) (resp *types.CreateFbAdsJobResponse, err error) {
	if req.Query == "" && req.PageId == "" {
		return nil, fmt.Errorf("either query or page_id is required")
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
		`INSERT INTO jobs (type, status, params) VALUES ('fbads', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	msg, err := json.Marshal(map[string]interface{}{
		"job_id":        jobID,
		"query":         req.Query,
		"page_id":       req.PageId,
		"country":       req.Country,
		"category":      req.Category,
		"media_type":    req.MediaType,
		"sort_by":       req.SortBy,
		"active_status": req.ActiveStatus,
		"min_date":      req.MinDate,
		"max_date":      req.MaxDate,
		"max_items":     req.MaxItems,
		"advertisers":   req.Advertisers,
		"fetch_details": req.FetchDetails,
	})
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, "fbads.jobs", msg); err != nil {
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &types.CreateFbAdsJobResponse{JobId: jobID}, nil
}
