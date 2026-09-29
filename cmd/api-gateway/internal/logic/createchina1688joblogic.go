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

type CreateChina1688JobLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateChina1688JobLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateChina1688JobLogic {
	return &CreateChina1688JobLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateChina1688JobLogic) CreateChina1688Job(req *types.CreateChina1688JobRequest) (resp *types.CreateChina1688JobResponse, err error) {
	if len(req.Keywords) == 0 && len(req.OfferIds) == 0 {
		return nil, fmt.Errorf("either keywords or offer_ids is required")
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
		`INSERT INTO jobs (type, status, params) VALUES ('china1688', 'pending', $1) RETURNING id`, params,
	).Scan(&jobID)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}

	msg, err := json.Marshal(map[string]interface{}{
		"job_id":    jobID,
		"keywords":  req.Keywords,
		"offer_ids": req.OfferIds,
		"max_items": req.MaxItems,
	})
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, "china1688.jobs", msg); err != nil {
		return nil, fmt.Errorf("publish job: %w", err)
	}

	return &types.CreateChina1688JobResponse{JobId: jobID}, nil
}
