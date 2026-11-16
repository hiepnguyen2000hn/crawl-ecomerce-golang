package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/pipiads"
)

func main() {
	keyword := flag.String("keyword", "car xiaomi", "Keyword to search ads on PiPiAds")
	limit := flag.Int("limit", 5, "Number of ads to display")
	flag.Parse()

	pgDSN := os.Getenv("POSTGRES_DSN")
	if pgDSN == "" {
		pgDSN = "postgres://crawl:crawl@localhost:5433/crawl?sslmode=disable"
	}

	conn, err := db.Connect(pgDSN)
	if err != nil {
		log.Fatalf("Lỗi kết nối Postgres: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	client := pipiads.NewHTTPClient("https://www.pipiads.com")
	registry := pipiads.NewAccountRegistry(conn, client)

	account, err := registry.AcquireAccount(ctx)
	if err != nil {
		log.Fatalf("Lỗi lấy account PiPiAds khả dụng: %v", err)
	}

	fmt.Printf("\n========================================================================================\n")
	fmt.Printf("👉 Sử dụng Account: %s\n", account.Email)
	fmt.Printf("👉 Proxy: %s\n", account.Proxy)
	fmt.Printf("👉 Credit trước khi search: %d/%d (Còn lại: %d)\n", account.UsedCredits, account.TotalCredits, account.TotalCredits-account.UsedCredits)
	fmt.Printf("========================================================================================\n\n")

	fmt.Printf("🔎 Đang tìm kiếm quảng cáo theo keyword: '%s'...\n\n", *keyword)

	ads, err := client.SearchEcommerceAds(ctx, account, pipiads.SearchRequest{
		Keyword:     *keyword,
		CurrentPage: 1,
		PageSize:    *limit,
	})
	if err != nil {
		log.Fatalf("Tìm kiếm thất bại: %v", err)
	}

	_ = registry.SyncAccountQuota(ctx, account)

	fmt.Printf("✅ TÌM THẤY %d SẢN PHẨM / QUẢNG CÁO (EXTRACT TOÀN DIỆN):\n", len(ads))
	fmt.Println("========================================================================================")

	for i, ad := range ads {
		desc := strings.TrimSpace(ad.Description)
		if desc == "" {
			desc = "(Không có text mô tả)"
		}

		brand := ad.AdvertiserName
		if brand == "" {
			brand = ad.AppName
		}
		if brand == "" {
			brand = "(N/A)"
		}

		fmt.Printf("\n[%d] VIDEO ID: %s | AD ID: %s\n", i+1, ad.VideoID, ad.AdID)
		fmt.Printf("    🏢 Nhà chạy Ads (Thương hiệu/Shop): %s (Handle: @%s)\n", brand, ad.UniqueID)
		fmt.Printf("    ⏱️  Thời lượng video: %d giây (%02d:%02d)\n", ad.Duration, ad.Duration/60, ad.Duration%60)
		fmt.Printf("    📊 Thống kê tiếp cận:\n")
		fmt.Printf("       • Lượt xem (Hiển thị): %d\n", ad.PlayCount)
		fmt.Printf("       • Lượt thích (Thả tim): %d\n", ad.DiggCount)
		fmt.Printf("       • Lượt bình luận: %d\n", ad.CommentCount)
		fmt.Printf("       • Lượt chia sẻ: %d\n", ad.ShareCount)

		if ad.FirstTime > 0 || ad.LastTime > 0 {
			firstStr := time.Unix(ad.FirstTime, 0).Format("2006-01-02")
			lastStr := time.Unix(ad.LastTime, 0).Format("2006-01-02")
			fmt.Printf("    📅 Thời gian chạy: Từ %s đến %s (Đã chạy %d ngày)\n", firstStr, lastStr, ad.PutDays)
		}

		if len(ad.FetchRegion) > 0 {
			fmt.Printf("    🌍 Quốc gia mục tiêu: %s\n", strings.Join(ad.FetchRegion, ", "))
		}

		fmt.Printf("    🎥 Link tải Video MP4: %s\n", ad.VideoURL)
		fmt.Printf("    🖼️  Ảnh bìa (Cover URL): %s\n", ad.CoverURL)

		if ad.ButtonText != "" {
			fmt.Printf("    🔘 Nút CTA: [%s]\n", ad.ButtonText)
		}
		if ad.LandingPageURL != "" {
			fmt.Printf("    🔗 Landing Page (Trang đích): %s\n", ad.LandingPageURL)
		}

		if ad.Script != "" && ad.Script != "null" {
			fmt.Printf("    🎙️ Kịch bản / Lời thoại AI trích xuất: %s\n", ad.Script)
		}
		if ad.MainHook != "" && ad.MainHook != "null" {
			fmt.Printf("    🪝 Hook chính mở đầu: %s\n", ad.MainHook)
		}
		if len(ad.Tags) > 0 {
			fmt.Printf("    🏷️  Hashtags: %s\n", strings.Join(ad.Tags, " "))
		}

		fmt.Printf("    📝 Mô tả caption: %s\n", desc)
		fmt.Println("----------------------------------------------------------------------------------------")
	}

	fmt.Printf("\n💰 Credit còn lại sau khi search: %d/%d (Còn lại: %d)\n\n", account.UsedCredits, account.TotalCredits, account.TotalCredits-account.UsedCredits)
}
