// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package handler

import (
	"net/http"

	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/logic"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/svc"
	"github.com/hiepnv/crawl-ecomerce-golang/cmd/api-gateway/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetAmazonJobHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetAmazonJobRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewGetAmazonJobLogic(r.Context(), svcCtx)
		resp, err := l.GetAmazonJob(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
