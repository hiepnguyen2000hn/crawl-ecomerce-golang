package logic

import (
	"context"
	"encoding/json"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
)

type CreatePipiadsJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreatePipiadsJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePipiadsJobLogic {
	return &CreatePipiadsJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreatePipiadsJobLogic) CreatePipiadsJob(req *types.CreatePipiadsJobRequest) (resp *types.CreatePipiadsJobResponse, err error) {
	params, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var jobID string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('pipiads', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, err
	}

	msg, err := json.Marshal(map[string]interface{}{
		"job_id":       jobID,
		"keyword":      req.Keyword,
		"country":      req.Country,
		"countries":    req.Countries,
		"language":     req.Language,
		"languages":    req.Languages,
		"search_type":  req.SearchType,
		"sort":         req.Sort,
		"sort_type":    req.SortType,
		"current_page": req.CurrentPage,
		"page_size":    req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	if err := l.svcCtx.Publisher.Publish(l.ctx, "crawl.pipiads", msg); err != nil {
		return nil, err
	}

	return &types.CreatePipiadsJobResponse{JobId: jobID}, nil
}
