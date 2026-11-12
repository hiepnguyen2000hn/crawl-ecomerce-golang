package logic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateKalodataJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateKalodataJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateKalodataJobLogic {
	return &CreateKalodataJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateKalodataJobLogic) CreateKalodataJob(req *types.CreateKalodataJobRequest) (resp *types.CreateKalodataJobResponse, err error) {
	params, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var jobID string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('kalodata', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	msg, err := json.Marshal(map[string]interface{}{
		"job_id":            jobID,
		"keyword":           req.Keyword,
		"country":           req.Country,
		"start_date":        req.StartDate,
		"end_date":          req.EndDate,
		"page_no":           req.PageNo,
		"page_size":         req.PageSize,
		"sort_field":        req.SortField,
		"sort_type":         req.SortType,
		"category_ids":      req.CategoryIds,
		"revenue":           req.Revenue,
		"unit_price":        req.UnitPrice,
		"sales_channel":     req.SalesChannel,
		"commission_rate":   req.CommissionRate,
		"creator":           req.Creator,
		"with_products":     req.WithProducts,
		"with_videos":       req.WithVideos,
		"with_video_detail": req.WithVideoDetail,
		"video_type":        req.VideoType,
		"duration":          req.Duration,
		"creator_followers": req.CreatorFollowers,
	})
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, "kalodata.jobs", msg); err != nil {
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &types.CreateKalodataJobResponse{JobId: jobID}, nil
}
