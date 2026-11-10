package logic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateLinkedInJobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateLinkedInJobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateLinkedInJobLogic {
	return &CreateLinkedInJobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateLinkedInJobLogic) CreateLinkedInJob(req *types.CreateLinkedInJobRequest) (resp *types.CreateLinkedInJobResponse, err error) {
	if req.Keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}

	params, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var jobID string
	err = l.svcCtx.DB.QueryRowContext(l.ctx,
		`INSERT INTO jobs (type, status, params) VALUES ('linkedin', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	msg, err := json.Marshal(map[string]interface{}{
		"job_id":            jobID,
		"keyword":           req.Keyword,
		"max_posts":         req.MaxPosts,
		"pages":             req.Pages,
		"sort_by":           req.SortBy,
		"posted_limit":      req.PostedLimit,
		"author_urls":       req.AuthorUrls,
		"authors_companies": req.AuthorsCompanies,
		"author_keywords":   req.AuthorKeywords,
		"content_type":      req.ContentType,
	})
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, "linkedin.jobs", msg); err != nil {
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &types.CreateLinkedInJobResponse{JobId: jobID}, nil
}
