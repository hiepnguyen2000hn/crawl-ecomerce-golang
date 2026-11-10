// Code scaffolded by goctl. Safe to edit.

package handler

import (
	"net/http"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/logic"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func CreateLinkedInJobHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateLinkedInJobRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewCreateLinkedInJobLogic(r.Context(), svcCtx)
		resp, err := l.CreateLinkedInJob(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
