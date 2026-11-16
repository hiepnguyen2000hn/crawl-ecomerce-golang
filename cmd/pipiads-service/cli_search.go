package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hiepnv/crawl-ecomerce-golang/pkg/db"
	"github.com/hiepnv/crawl-ecomerce-golang/pkg/pipiads"
)

func main() {
	keyword := flag.String("keyword", "shoes", "Keyword to search ads on PiPiAds")
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

	// Tự động acquire account khả dụng và có quota
	account, err := registry.AcquireAccount(ctx)
	if err != nil {
		log.Fatalf("Lỗi lấy account PiPiAds khả dụng: %v", err)
	}

	fmt.Printf("\n======================================================\n")
	fmt.Printf("👉 Sử dụng Account: %s\n", account.Email)
	fmt.Printf("👉 Device ID: %s | Proxy: %s\n", account.DeviceID, account.Proxy)
	fmt.Printf("👉 Credit trước khi search: %d/%d (Còn lại: %d)\n", account.UsedCredits, account.TotalCredits, account.TotalCredits-account.UsedCredits)
	fmt.Printf("======================================================\n\n")

	fmt.Printf("🔎 Đang tìm kiếm quảng cáo theo keyword: '%s'...\n\n", *keyword)

	ads, err := client.SearchEcommerceAds(ctx, account, pipiads.SearchRequest{
		Keyword:     *keyword,
		CurrentPage: 1,
		PageSize:    *limit,
	})
	if err != nil {
		log.Fatalf("Tìm kiếm thất bại: %v", err)
	}

	// Cập nhật lại số credits thực tế sau khi search
	_ = registry.SyncAccountQuota(ctx, account)

	fmt.Printf("✅ TÌM THẤY %d SẢN PHẨM/QUẢNG CÁO:\n", len(ads))
	fmt.Println("-------------------------------------------------------------------------------------------------------")

	for i, ad := range ads {
		desc := ad.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		if desc == "" {
			desc = "(Không có mô tả text)"
		}

		brand := ad.AdvertiserName
		if brand == "" {
			brand = ad.AppName
		}
		if brand == "" {
			brand = "(N/A)"
		}

		fmt.Printf("[%d] Video ID: %s | Ad ID: %s\n", i+1, ad.VideoID, ad.AdID)
		fmt.Printf("    🏷️  Thương hiệu/Shop: %s\n", brand)
		fmt.Printf("    📊 Thống kê: %d Lượt xem | %d Lượt thích | %d Bình luận | %d Chia sẻ\n", ad.PlayCount, ad.DiggCount, ad.CommentCount, ad.ShareCount)
		fmt.Printf("    📝 Mô tả: %s\n", desc)
		if ad.CoverURL != "" {
			fmt.Printf("    🖼️  Ảnh/Cover: %s\n", ad.CoverURL)
		}
		if ad.LandingPageURL != "" {
			fmt.Printf("    🔗 Landing Page: %s\n", ad.LandingPageURL)
		}
		fmt.Println("-------------------------------------------------------------------------------------------------------")
	}

	fmt.Printf("\n💰 Credit còn lại sau khi search: %d/%d (Còn lại: %d)\n\n", account.UsedCredits, account.TotalCredits, account.TotalCredits-account.UsedCredits)
}
