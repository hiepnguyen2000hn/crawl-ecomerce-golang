# Từ điển dữ liệu (Data Dictionary)

Tài liệu này giải thích **toàn bộ các cột dữ liệu** trong:

1. [Cơ sở dữ liệu PostgreSQL](#1-cơ-sở-dữ-liệu-postgresql) (`db/migrations/*.up.sql`)
2. [File export từ DB](#2-file-export-từ-db-exports) (`exports/*.csv`, `exports/crawl_data.xlsx`)
3. [File export từ live test theo service](#3-file-export-theo-service-exportscrawl_ngày_keyword_geo) (`exports/crawl_<ngày>_<keyword>_<geo>/`)
4. [Bản export tiếng Việt](#4-bản-export-tiếng-việt-exportscrawl__tieng_viet) (`exports/crawl_<...>_tieng_viet/`)
5. [Dữ liệu thô](#5-dữ-liệu-thô) (`exports/live/*.json`, `exports/live/page_*.md`, dataset Apify)

---

## Tổng quan luồng dữ liệu

```
POST /trend  ──► jobs(type=trend) ──► trend-service  ──► trend_raw ──┐
                                        (SerpAPI Google Trends)      ├─► ai-service ──► ai_results
POST /fbads  ──► jobs(type=fbads) ──► fb-ads-service ──► fbads_raw ──┘
                                        (Apify FB Ad Library)  │
                                                               └─► product-extractor-service ──► products
                                                                     (crawl4ai + OpenRouter)
niche-research-service (đồng bộ) ──► niche_sessions  (OpenRouter + Google Trends 24 tháng)
```

Vòng đời `jobs.status`: `pending` → `crawled` (crawler ghi xong dữ liệu thô) → `ai_processing` → `done`; lỗi ở bất kỳ bước nào → `failed`.

---

## 1. Cơ sở dữ liệu PostgreSQL

Mọi khoá chính là `UUID` sinh tự động bằng `gen_random_uuid()` (extension `pgcrypto`). Thời gian là `TIMESTAMPTZ` (lưu theo UTC).

### 1.1. `jobs` — một lượt yêu cầu crawl

| Cột | Kiểu | Ý nghĩa |
|---|---|---|
| `id` | UUID, PK | Mã job. Các bảng `trend_raw`, `fbads_raw`, `products`, `ai_results` tham chiếu qua `job_id`. |
| `type` | TEXT | Loại job: `trend` (Google Trends) hoặc `fbads` (Facebook Ad Library). |
| `status` | TEXT | Trạng thái: `pending` (vừa tạo) · `crawled` (đã cào xong dữ liệu thô) · `ai_processing` (AI đang phân tích) · `done` (hoàn tất) · `failed` (lỗi). |
| `params` | JSONB | Tham số người dùng gửi lên. Xem bảng bên dưới. |
| `created_at` | TIMESTAMPTZ | Thời điểm tạo job. |
| `updated_at` | TIMESTAMPTZ | Lần cập nhật trạng thái gần nhất. `updated_at - created_at` ≈ thời gian xử lý. |

**`params` khi `type = trend`:**

| Khoá | Ý nghĩa |
|---|---|
| `keyword` | Từ khoá tra Google Trends. |
| `geo` | Mã quốc gia (vd `US`); rỗng = toàn cầu (WORLD). |

**`params` khi `type = fbads`** (khớp input của Apify actor Facebook Ad Library Scraper):

| Khoá | Ý nghĩa |
|---|---|
| `query` | Từ khoá tìm quảng cáo. |
| `country` | Quốc gia quảng cáo được phân phối (vd `US`). |
| `page_id` | Chỉ lấy quảng cáo của một Fanpage cụ thể. |
| `category` | Nhóm quảng cáo (vd tất cả / chính trị…). |
| `media_type` | Lọc theo loại media (ảnh, video…). |
| `sort_by` | Cách sắp xếp kết quả. |
| `active_status` | Lọc quảng cáo đang chạy / đã dừng / tất cả. |
| `min_date`, `max_date` | Khoảng ngày bắt đầu chạy quảng cáo. |
| `max_items` | Số quảng cáo tối đa; `0` = mặc định của client (50). |
| `advertisers` | Danh sách nhà quảng cáo cần lọc (null = không lọc). |
| `fetch_details` | `true` = lấy thêm chi tiết từng quảng cáo (chậm hơn). |

### 1.2. `trend_raw` — dữ liệu thô Google Trends

| Cột | Kiểu | Ý nghĩa |
|---|---|---|
| `id` | UUID, PK | Mã bản ghi. |
| `job_id` | UUID, FK → `jobs.id` | Job sinh ra bản ghi. |
| `keyword` | TEXT | Từ khoá đã tra. |
| `trend_data` | JSONB | Nguyên response SerpAPI `engine=google_trends`. Cấu trúc xem [mục 5.1](#51-trend__json--niche_trend24m__json-response-serpapi-google-trends). |
| `fetched_at` | TIMESTAMPTZ | Thời điểm lấy dữ liệu. |

### 1.3. `fbads_raw` — quảng cáo Facebook (Meta Ad Library)

Mỗi dòng là một quảng cáo. Các cột phẳng được trích từ `raw` cho tiện truy vấn.

| Cột | Kiểu | Ý nghĩa |
|---|---|---|
| `id` | UUID, PK | Mã nội bộ của quảng cáo (dùng bởi `products.ad_id`). |
| `job_id` | UUID, FK → `jobs.id` | Job crawl. |
| `ad_archive_id` | TEXT | Mã quảng cáo trong Thư viện quảng cáo Meta. Link xem: `https://www.facebook.com/ads/library/?id=<ad_archive_id>`. |
| `page_id` | TEXT | ID Fanpage chạy quảng cáo. |
| `page_name` | TEXT | Tên Fanpage. |
| `is_active` | BOOLEAN | Quảng cáo còn đang chạy tại thời điểm crawl. |
| `start_date` | TIMESTAMPTZ | Ngày bắt đầu chạy. |
| `end_date` | TIMESTAMPTZ | Ngày kết thúc (với quảng cáo đang chạy thường là ngày crawl). |
| `body` | TEXT | Nội dung chữ chính (primary text) của quảng cáo. |
| `title` | TEXT | Tiêu đề (headline) hiển thị dưới ảnh/video. |
| `cta_text` | TEXT | Chữ trên nút kêu gọi hành động, vd `Shop now`. |
| `cta_type` | TEXT | Mã loại nút CTA, vd `SHOP_NOW`, `DOWNLOAD`, `MESSAGE_PAGE`, `LEARN_MORE`, `WHATSAPP_MESSAGE`… |
| `link_url` | TEXT | URL trang đích (landing page) khi bấm quảng cáo. Là đầu vào của product-extractor. |
| `raw` | JSONB | Nguyên object Apify trả về. Cấu trúc xem [mục 5.2](#52-fbads__json--dataset-apify). |
| `fetched_at` | TIMESTAMPTZ | Thời điểm ghi vào DB. |

### 1.4. `products` — sản phẩm bóc từ trang đích của quảng cáo

product-extractor-service tải `link_url` bằng crawl4ai (ra Markdown), rồi nhờ AI (OpenRouter) trích sản phẩm. **Chỉ sản phẩm có giá ghi rõ trên trang mới được lưu.**

| Cột | Kiểu | Ý nghĩa |
|---|---|---|
| `id` | UUID, PK | Mã sản phẩm. |
| `job_id` | UUID, FK → `jobs.id` | Job fbads gốc. |
| `ad_id` | UUID, FK → `fbads_raw.id` | Quảng cáo dẫn tới trang chứa sản phẩm. |
| `url` | TEXT | URL trang đích đã crawl. |
| `product_name` | TEXT | Tên sản phẩm (AI trích). |
| `price` | NUMERIC | Giá **đúng như hiển thị trên trang**, không quy đổi. |
| `currency` | TEXT | Mã tiền tệ ISO của giá, vd `USD`, `VND`. Lưu ý: một số shop Shopify hiển thị theo tiền tệ của IP người crawl (vd `VND`) chứ không phải tiền gốc của shop. |
| `sku` | TEXT | Mã sản phẩm. Nếu trang không ghi SKU, AI tự sinh một định danh ngắn từ tên (thường giống slug URL). |
| `raw` | JSONB | Object sản phẩm AI trả về. |
| `created_at` | TIMESTAMPTZ | Thời điểm ghi. |

### 1.5. `ai_results` — kết quả phân tích AI cho một job

| Cột | Kiểu | Ý nghĩa |
|---|---|---|
| `id` | UUID, PK | Mã kết quả. |
| `job_id` | UUID, FK → `jobs.id` | Job được phân tích. |
| `provider` | TEXT | Nhà cung cấp AI, hiện luôn là `openrouter`. |
| `model` | TEXT | Model đã dùng, vd `dots-studio/dots-3-note-preview:free`. |
| `prompt_type` | TEXT | `analysis` (tóm tắt/phân tích) hoặc `generate` (sinh nội dung — chưa dùng). |
| `input_ref` | UUID | Tham chiếu tới bản ghi đầu vào cụ thể (hiện để trống). |
| `output` | JSONB | `{"content": "<văn bản AI>"}`. Với trend: 2–3 câu nhận định xu hướng; với fbads: 3–4 câu về pattern, thông điệp, CTA chung của các quảng cáo. |
| `created_at` | TIMESTAMPTZ | Thời điểm ghi. |

### 1.6. `niche_sessions` — phiên nghiên cứu ngách (Bước 1: Nhu cầu thị trường)

Tạo bởi niche-research-service. Không gắn với `jobs`.

| Cột | Kiểu | Ý nghĩa |
|---|---|---|
| `id` | UUID, PK | Mã phiên. |
| `raw_keyword` | TEXT | Từ khoá người dùng nhập (có thể tiếng Việt). |
| `countries` | JSONB | Danh sách thị trường `[{"code":"US","name":"Mỹ"}]`. Phần tử đầu tiên là thị trường chính dùng để phân tích. |
| `status` | TEXT | `draft` (mặc định) / `done`. |
| `step1_score` | NUMERIC | **Điểm Bước 1 (thang 0–10)** = trung bình của S1.2 và S1.3 (xem cách tính bên dưới), làm tròn 1 chữ số. |
| `avg_monthly_searches` | INTEGER | ⚠️ Tên cột dễ hiểu nhầm: **không phải lượt tìm kiếm tuyệt đối**, mà là **chỉ số quan tâm Google Trends trung bình (0–100)** của 12 tháng gần nhất. |
| `demand_type` | TEXT | Loại nhu cầu: `evergreen` (ổn định quanh năm) · `seasonal` (theo mùa) · `spike` (trend ngắn hạn). |
| `demand_label` | TEXT | Mô tả tiếng Việt của `demand_type`. |
| `kw_source` | TEXT | Nguồn từ khoá bản địa hoá: `openrouter` (AI) hoặc `demo` (AI lỗi, dùng từ khoá gốc). |
| `volume_source` | TEXT | Nguồn dữ liệu nhu cầu: `google_trends_index` hoặc `demo` (không lấy được Trends). |
| `ai_source` | TEXT | Nguồn phân tích AI: `openrouter` hoặc `demo`. |
| `peak_months` | JSONB | Tháng có chỉ số cao nhất trong 12 tháng gần nhất, vd `["Apr"]`. |
| `low_months` | JSONB | Tháng có chỉ số thấp nhất, vd `["Jan"]`. |
| `trend_direction` | TEXT | Xu hướng trong 12 tháng gần nhất: so trung bình nửa sau với nửa đầu — `up` (> +10%), `down` (< −10%), `stable`. |
| `volatility` | TEXT | Mức biến động mùa vụ: `low` / `medium` / `high`. |
| `volatility_label` | TEXT | Mô tả tiếng Việt: Biến động thấp / trung bình / cao. |
| `fluctuation_ratio` | NUMERIC | ⚠️ **Tỷ lệ tăng/giảm so với cùng kỳ năm trước (YoY)** = (TB 12 tháng gần nhất − TB 12 tháng trước đó) / TB 12 tháng trước. Vd `0.4166` = **+41,7%**. (Tỷ lệ biến động mùa vụ dùng để phân loại được ghi trong `seasonality_note`, không phải cột này.) |
| `seasonality_note` | TEXT | Ghi chú giải thích điểm S1 (chỉ số trung bình, % biến động mùa vụ, các tiêu chí chưa tính) hoặc lý do lỗi. |
| `ai_summary` | TEXT | Tóm tắt thị trường 2–3 câu (tiếng Việt) do AI viết. |
| `ai_risks` | JSONB | Mảng 2–3 rủi ro (tiếng Việt). |
| `ai_actions` | JSONB | Mảng 2–3 hành động đề xuất tiếp theo (tiếng Việt). |
| `last_run_at` | TIMESTAMPTZ | Lần chạy phân tích gần nhất. |
| `created_at` | TIMESTAMPTZ | Thời điểm tạo phiên. |

**Cách tính điểm Bước 1** (từ 12 tháng gần nhất của chuỗi Google Trends 24 tháng):

- **Tỷ lệ biến động mùa vụ** `r = (max − min) / max`
  - `r ≤ 0.30` → `evergreen`, `low`, **S1.3 = 9.5**
  - `r ≤ 0.70` → `seasonal`, `medium`, **S1.3 = 6**
  - `r > 0.70` → `spike`, `high`, **S1.3 = 2.5**
- **Mức nhu cầu (S1.2, proxy)** theo chỉ số trung bình `avg`: `≥50 → 9`, `≥25 → 7`, `≥10 → 5`, còn lại `2`.
- `step1_score = round((S1.2 + S1.3) / 2, 1)`.
- S1.1 (quy mô tệp khách Meta) và S1.4 (tiếng nói khách hàng Reddit) **chưa được tính** vì chưa có credential API.

---

## 2. File export từ DB (`exports/`)

Sinh bởi `scripts/export_data.sh` (dump trực tiếp từ Postgres). `exports/crawl_data.xlsx` gộp 6 CSV này thành 6 sheet cùng tên: `jobs`, `trend`, `fbads`, `products`, `ai_results`, `niche_sessions`.

| File | Nguồn | Cột |
|---|---|---|
| `jobs.csv` | `jobs` | Giống bảng [1.1](#11-jobs--một-lượt-yêu-cầu-crawl); `params` ở dạng chuỗi JSON. |
| `trend.csv` | `trend_raw` | Giống bảng [1.2](#12-trend_raw--dữ-liệu-thô-google-trends). |
| `fbads.csv` | `fbads_raw` ⨝ `jobs` | `id, job_id, query, ad_archive_id, page_id, page_name, is_active, start_date, end_date, title, body, cta_text, cta_type, link_url, fetched_at` — như bảng [1.3](#13-fbads_raw--quảng-cáo-facebook-meta-ad-library) (bỏ `raw`), thêm `query` = `jobs.params.query`. `is_active` hiển thị `t`/`f`. |
| `products.csv` | `products` ⟕ `fbads_raw` | `id, job_id, ad_id, page_name, product_name, price, currency, sku, url, created_at` — như bảng [1.4](#14-products--sản-phẩm-bóc-từ-trang-đích-của-quảng-cáo), thêm `page_name` của quảng cáo gốc. |
| `ai_results.csv` | `ai_results` ⨝ `jobs` | Như bảng [1.5](#15-ai_results--kết-quả-phân-tích-ai-cho-một-job), thêm `job_type` = `jobs.type`. |
| `niche_sessions.csv` | `niche_sessions` | Như bảng [1.6](#16-niche_sessions--phiên-nghiên-cứu-ngách-bước-1-nhu-cầu-thị-trường) (không có `volatility_label`); các cột JSONB ở dạng chuỗi JSON. |

---

## 3. File export theo service (`exports/crawl_<ngày>_<keyword>_<geo>/`)

Sinh bởi `scripts/export_live.py` từ JSON thô trong `exports/live/` (do live test lưu). Mỗi thư mục con có CSV và một file `<service>.xlsx` gộp các CSV đó (mỗi CSV một sheet).

### 3.1. `trend-service/`

**`trend_weekly.csv`** — chỉ số Google Trends theo tuần, 12 tháng gần nhất.

| Cột | Ý nghĩa |
|---|---|
| `keyword` | Từ khoá. |
| `geo` | Khu vực (`WORLD` nếu toàn cầu). |
| `date_range` | Khoảng thời gian tra, vd `today 12-m` = 12 tháng gần nhất. |
| `week_label` | Nhãn tuần do Google Trends trả về, vd `Sep 21 – 27, 2025`. |
| `week_start` | Ngày đầu tuần (YYYY-MM-DD, UTC). |
| `value` | **Chỉ số quan tâm tương đối 0–100**: 100 = tuần có mức tìm kiếm cao nhất trong khoảng, 50 = bằng một nửa, 0 = không đủ dữ liệu. Không phải số lượt tìm kiếm. |
| `partial_data` | `True` = tuần hiện tại chưa kết thúc, số liệu chưa đầy đủ. |
| `fetched_at` | Thời điểm SerpAPI lấy dữ liệu. |

**`trend_monthly.csv`** — gộp tuần thành tháng (bỏ tuần `partial_data`).

| Cột | Ý nghĩa |
|---|---|
| `keyword`, `geo` | Như trên. |
| `month` | Tháng (YYYY-MM), xác định theo ngày đầu tuần. |
| `avg_value` | Trung bình chỉ số 0–100 của các tuần trong tháng. |
| `weeks` | Số tuần được tính vào trung bình (tháng đầu/cuối có thể ít tuần). |

### 3.2. `fb-ads-service/fbads.csv`

| Cột | Ý nghĩa |
|---|---|
| `query` | Từ khoá tìm quảng cáo. |
| `country` | Quốc gia. |
| `ad_archive_id` | Mã quảng cáo trong Thư viện quảng cáo Meta. |
| `ad_library_url` | Link mở quảng cáo trên Ad Library. |
| `page_id` | ID Fanpage. |
| `page_name` | Tên Fanpage. |
| `page_like_count` | Số lượt thích Fanpage (page mới lập thường rất thấp — dấu hiệu page dropship). |
| `page_profile_uri` | Link Fanpage. |
| `is_active` | `True` = còn đang chạy. |
| `start_date` / `end_date` | Ngày bắt đầu / kết thúc (YYYY-MM-DD). |
| `total_active_days` | Tổng thời gian quảng cáo đã chạy, tính bằng **ngày** (Meta đếm thời gian thực sự phân phối, nên có thể < khoảng `start_date`→`end_date`). Quảng cáo chạy càng lâu thường càng hiệu quả. |
| `impressions` | Khoảng lượt hiển thị Meta công bố, vd `<100`. Thường trống — Meta chỉ công bố cho quảng cáo chính trị/xã hội hoặc tại EU. |
| `reach_estimate` | Ước tính lượt tiếp cận (thường trống, lý do như trên). |
| `spend` | Chi tiêu quảng cáo (thường trống, lý do như trên). |
| `publisher_platforms` | Nền tảng hiển thị: `FACEBOOK`, `INSTAGRAM`, `AUDIENCE_NETWORK`, `MESSENGER`, `WHATSAPP`, `THREADS`. |
| `display_format` | Định dạng: `IMAGE` (1 ảnh), `VIDEO`, `CAROUSEL` (nhiều thẻ trượt), `MULTI_IMAGES`, `DCO` (Dynamic Creative — Meta tự phối ảnh/chữ), `DPA` (Dynamic Product Ads — quảng cáo sản phẩm động từ catalog). |
| `title` | Tiêu đề (headline). |
| `body` | Nội dung chữ chính. |
| `caption` | Chú thích dưới ảnh, thường là tên miền trang đích. |
| `link_description` | Mô tả link (dòng phụ dưới tiêu đề). |
| `cta_text` / `cta_type` | Chữ / mã loại nút kêu gọi hành động (xem [1.3](#13-fbads_raw--quảng-cáo-facebook-meta-ad-library)). |
| `link_url` | URL trang đích. |
| `link_domain` | Tên miền của `link_url` (bỏ `www.`). |
| `collation_count` | Số phiên bản quảng cáo Meta gộp chung (cùng nội dung, khác biến thể). Nhiều phiên bản = nhà quảng cáo đang test/scale. |
| `image_count` | Số ảnh trong quảng cáo. |
| `video_count` | Số video trong quảng cáo. |

### 3.3. `product-extractor-service/`

**`products.csv`** — mỗi dòng một sản phẩm.

| Cột | Ý nghĩa |
|---|---|
| `landing_url` | URL trang đích đã crawl. |
| `landing_domain` | Tên miền trang đích. |
| `page_names` | Các Fanpage có quảng cáo dẫn tới URL này (phân tách bằng dấu phẩy). |
| `ad_archive_ids` | Các mã quảng cáo dẫn tới URL này. |
| `product_name` | Tên sản phẩm. |
| `price` | Giá như hiển thị trên trang. |
| `currency` | Mã tiền tệ. |
| `sku` | Mã sản phẩm (AI tự sinh nếu trang không có). |

**`landing_pages.csv`** — mỗi dòng một trang đích, kết quả crawl.

| Cột | Ý nghĩa |
|---|---|
| `landing_url`, `landing_domain`, `page_names` | Như trên. |
| `ad_count` | Số quảng cáo dẫn tới trang này. |
| `status` | `products_found` (có sản phẩm) · `no_products` (không phải trang bán hàng hoặc không ghi giá) · `error` (crawl/AI lỗi). |
| `product_count` | Số sản phẩm bóc được. |
| `error` | Thông báo lỗi (khi `status = error`). |

### 3.4. `niche-research-service/`

| File | Cột |
|---|---|
| `niche_session.csv` | Như bảng [1.6](#16-niche_sessions--phiên-nghiên-cứu-ngách-bước-1-nhu-cầu-thị-trường), nhưng `countries` được tách thành `country_code` + `country_name` (thị trường chính), `peak_months`/`low_months` dạng chuỗi phân tách dấu phẩy; không có `ai_risks`/`ai_actions` (xem file dưới) và `created_at`. |
| `niche_ai_insights.csv` | `type` (`risk` = rủi ro, `action` = hành động đề xuất) · `no` (số thứ tự) · `text` (nội dung). |
| `niche_trend_24m_weekly.csv` | Như `trend_weekly.csv` nhưng khoảng 24 tháng (`date_range` dạng `YYYY-MM-DD YYYY-MM-DD`) và không có `fetched_at`. |
| `niche_trend_24m_monthly.csv` | Như `trend_monthly.csv`, khoảng 24 tháng. |

---

## 4. Bản export tiếng Việt (`exports/crawl_<...>_tieng_viet/`)

Cùng dữ liệu với [mục 3](#3-file-export-theo-service-exportscrawl_ngày_keyword_geo) nhưng tên cột, giá trị và nội dung được Việt hoá (ngày dạng `DD/MM/YYYY`, `True/False` → `Có/Không`, nội dung quảng cáo/tên sản phẩm được dịch). Bảng đối chiếu:

### 4.1. `1_xu_huong_google_trends/` (= `trend-service/`)

| File tiếng Việt | Cột tiếng Việt → cột gốc |
|---|---|
| `xu_huong_theo_tuan.csv` | Từ khóa → `keyword` · Khu vực → `geo` · Khoảng thời gian → `date_range` · Tuần → `week_label` · Ngày đầu tuần → `week_start` · Chỉ số quan tâm (0–100) → `value` · Dữ liệu chưa đủ tuần → `partial_data` · Thời điểm lấy dữ liệu → `fetched_at` |
| `xu_huong_theo_thang.csv` | Từ khóa → `keyword` · Khu vực → `geo` · Tháng (`MM/YYYY`) → `month` · Chỉ số trung bình (0–100) → `avg_value` · Số tuần → `weeks` |

### 4.2. `2_quang_cao_facebook/quang_cao_facebook.csv` (= `fb-ads-service/fbads.csv`)

| Cột tiếng Việt | Cột gốc |
|---|---|
| Từ khóa tìm kiếm | `query` |
| Quốc gia | `country` |
| Mã quảng cáo | `ad_archive_id` |
| Link Thư viện quảng cáo | `ad_library_url` |
| Mã trang | `page_id` |
| Tên trang | `page_name` |
| Số lượt thích trang | `page_like_count` |
| Link trang Facebook | `page_profile_uri` |
| Đang chạy (`Có`/`Không`) | `is_active` |
| Ngày bắt đầu / Ngày kết thúc | `start_date` / `end_date` |
| Tổng số ngày đã chạy | `total_active_days` |
| Lượt hiển thị | `impressions` |
| Lượt tiếp cận ước tính | `reach_estimate` |
| Chi tiêu | `spend` |
| Nền tảng hiển thị | `publisher_platforms` |
| Định dạng quảng cáo | `display_format` |
| Tiêu đề | `title` |
| Nội dung quảng cáo | `body` |
| Chú thích (tên miền) | `caption` |
| Mô tả link | `link_description` |
| Chữ trên nút kêu gọi | `cta_text` |
| Loại nút kêu gọi | `cta_type` |
| Link đích | `link_url` |
| Tên miền đích | `link_domain` |
| Số phiên bản quảng cáo | `collation_count` |
| Số ảnh / Số video | `image_count` / `video_count` |

### 4.3. `3_san_pham_tu_trang_dich/` (= `product-extractor-service/`)

| File tiếng Việt | Cột tiếng Việt → cột gốc |
|---|---|
| `san_pham.csv` | Link trang đích → `landing_url` · Tên miền trang đích → `landing_domain` · Các trang quảng cáo → `page_names` · Các mã quảng cáo → `ad_archive_ids` · Tên sản phẩm → `product_name` · Giá → `price` · Tiền tệ → `currency` · Mã sản phẩm (SKU) → `sku` |
| `trang_dich.csv` | Link trang đích → `landing_url` · Tên miền trang đích → `landing_domain` · Các trang quảng cáo → `page_names` · Số quảng cáo dẫn tới → `ad_count` · Trạng thái (`Có sản phẩm` / `Không có sản phẩm` / `Lỗi`) → `status` · Số sản phẩm → `product_count` · Lỗi → `error` |

### 4.4. `4_nghien_cuu_ngach/` (= `niche-research-service/`)

**`phien_nghien_cuu.csv`** (= `niche_session.csv`)

| Cột tiếng Việt | Cột gốc |
|---|---|
| Mã phiên | `id` |
| Từ khóa gốc | `raw_keyword` |
| Mã quốc gia / Tên quốc gia | `country_code` / `country_name` |
| Trạng thái (`Hoàn tất`) | `status` |
| Điểm bước 1 (S1) | `step1_score` |
| Chỉ số tìm kiếm trung bình (Google Trends, 0–100) | `avg_monthly_searches` |
| Loại nhu cầu (`Ổn định` / `Theo mùa` / `Đột biến`) | `demand_type` |
| Mô tả nhu cầu | `demand_label` |
| Mức biến động (`Thấp` / `Trung bình` / `Cao`) | `volatility` |
| Mô tả biến động | `volatility_label` |
| Tỷ lệ dao động | `fluctuation_ratio` (tỷ lệ YoY — xem [1.6](#16-niche_sessions--phiên-nghiên-cứu-ngách-bước-1-nhu-cầu-thị-trường)) |
| Xu hướng (`Tăng` / `Giảm` / `Ổn định`) | `trend_direction` |
| Nguồn từ khóa / Nguồn lượng tìm kiếm / Nguồn phân tích AI | `kw_source` / `volume_source` / `ai_source` |
| Ghi chú mùa vụ | `seasonality_note` |
| Tóm tắt của AI | `ai_summary` |
| Lần chạy gần nhất | `last_run_at` |
| Tháng cao điểm / Tháng thấp điểm | `peak_months` / `low_months` |

**`goi_y_cua_ai.csv`** (= `niche_ai_insights.csv`): Loại (`Rủi ro` / `Hành động`) → `type` · STT → `no` · Nội dung → `text`.

**`xu_huong_24_thang_theo_tuan.csv`** / **`xu_huong_24_thang_theo_thang.csv`**: như [4.1](#41-1_xu_huong_google_trends--trend-service), khoảng 24 tháng.

---

## 5. Dữ liệu thô

### 5.1. `trend_*.json` / `niche_trend24m_*.json` (response SerpAPI Google Trends)

Cũng chính là nội dung cột `trend_raw.trend_data`.

| Trường | Ý nghĩa |
|---|---|
| `search_metadata.id`, `.status`, `.created_at`, `.processed_at`, `.total_time_taken` | Thông tin lượt gọi SerpAPI (mã, trạng thái, thời điểm, thời gian xử lý giây). |
| `search_metadata.json_endpoint` / `raw_html_file` / `google_trends_url` … | Link xem lại kết quả trên SerpAPI / Google Trends. |
| `search_parameters.q` | Từ khoá. |
| `search_parameters.geo` | Khu vực (trống = toàn cầu). |
| `search_parameters.date` | Khoảng thời gian (`today 12-m` hoặc `YYYY-MM-DD YYYY-MM-DD`). |
| `search_parameters.hl`, `.tz` | Ngôn ngữ, múi giờ (phút lệch UTC). |
| `search_parameters.engine`, `.data_type` | `google_trends`, `TIMESERIES`. |
| `interest_over_time.timeline_data[]` | Mảng các tuần: `date` (nhãn tuần), `timestamp` (Unix, đầu tuần), `partial_data`, `values[]` gồm `query`, `value` (chuỗi), `extracted_value` (số 0–100). |

### 5.2. `fbads_*.json` / dataset Apify

`exports/live/fbads_<query>_<country>_<stamp>.json` là mảng object Apify; `exports/dataset_facebook-ad-library-scraper_*.csv` là cùng dữ liệu được Apify làm phẳng (98 cột, đường dẫn phân tách bằng `/`, chỉ số mảng dạng `/0`, `/1`…). Cũng là nội dung cột `fbads_raw.raw`.

**Cấp quảng cáo**

| Trường | Ý nghĩa |
|---|---|
| `ad_archive_id` | Mã quảng cáo trong Ad Library. |
| `ad_id` | Mã quảng cáo nội bộ Meta (thường trống). |
| `collation_id`, `collation_count` | Mã nhóm và số phiên bản quảng cáo được gộp chung. |
| `categories/N` | Nhóm quảng cáo (vd `UNKNOWN`, `POLITICAL`…). |
| `page_id`, `page_name`, `page_is_deleted`, `is_profile_page` | Thông tin Fanpage chạy quảng cáo. |
| `is_active` | Còn đang chạy. |
| `start_date`, `end_date` | Unix timestamp bắt đầu / kết thúc. |
| `total_active_time` | Tổng thời gian đã phân phối, **tính bằng giây**. |
| `publisher_platform/N` | Nền tảng hiển thị. |
| `impressions_with_index/impressions_text`, `/impressions_index` | Khoảng lượt hiển thị (vd `<100`) và chỉ số bậc tương ứng. |
| `reach_estimate`, `spend`, `currency` | Tiếp cận, chi tiêu, tiền tệ chi tiêu (chỉ có với quảng cáo chính trị/EU). |
| `targeted_or_reached_countries` | Quốc gia nhắm tới/tiếp cận. |
| `contains_digital_created_media` | Có nội dung do AI/kỹ thuật số tạo. |
| `contains_sensitive_content`, `has_user_reported`, `report_count`, `hidden_safety_data`, `hide_data_status`, `gated_type` | Cờ nội dung nhạy cảm / bị báo cáo / bị ẩn. |
| `regional_regulation_data/...` | Cờ quy định khu vực (tài chính, chống lừa đảo Đài Loan…). |
| `is_aaa_eligible` | Đủ điều kiện Advantage+ (quảng cáo tự động của Meta). |
| `entity_type`, `fev_info`, `state_media_run_label`, `menu_items` | Trường phụ của Meta (loại thực thể, nhãn truyền thông nhà nước…), thường trống. |

**`snapshot/...` — nội dung sáng tạo của quảng cáo**

| Trường | Ý nghĩa |
|---|---|
| `snapshot/body/text` | Nội dung chữ chính. |
| `snapshot/title` | Tiêu đề. |
| `snapshot/caption` | Chú thích (thường là tên miền). |
| `snapshot/link_url`, `snapshot/link_description` | URL và mô tả trang đích. |
| `snapshot/cta_text`, `snapshot/cta_type` | Nút kêu gọi hành động. |
| `snapshot/display_format` | Định dạng (`IMAGE`, `VIDEO`, `DCO`, `DPA`, `CAROUSEL`, `MULTI_IMAGES`). |
| `snapshot/images/N/original_image_url`, `resized_image_url`, `watermarked_resized_image_url` | Link ảnh gốc / thu nhỏ / có watermark. |
| `snapshot/videos/N/video_hd_url`, `video_sd_url`, `video_preview_image_url`, `watermarked_video_*_url` | Link video HD/SD, ảnh preview, bản có watermark. |
| `snapshot/cards`, `extra_images`, `extra_videos`, `extra_texts`, `extra_links` | Các thẻ carousel và biến thể bổ sung (DCO). |
| `snapshot/page_name`, `page_id`, `page_like_count`, `page_profile_uri`, `page_profile_picture_url`, `page_categories/N`, `page_entity_type`, `page_is_deleted`, `page_is_profile_page`, `current_page_name` | Thông tin Fanpage tại thời điểm chụp quảng cáo. |
| `snapshot/byline`, `disclaimer_label` | Dòng "Được tài trợ bởi" (quảng cáo chính trị). |
| `snapshot/branded_content` | Thông tin nội dung hợp tác thương hiệu. |
| `snapshot/country_iso_code` | Mã quốc gia của quảng cáo. |
| `snapshot/event` | Thông tin sự kiện (nếu là quảng cáo sự kiện). |
| `snapshot/is_reshared`, `root_reshared_post` | Có phải bài chia sẻ lại và bài gốc. |
| `snapshot/additional_info`, `brazil_tax_id`, `ec_certificates` | Trường phụ theo quốc gia, thường trống. |

### 5.3. `products_extracted_*.json`

Object có khoá là tên trang mẫu (vd `shopify-empress`) hoặc URL; mỗi giá trị gồm:

| Trường | Ý nghĩa |
|---|---|
| `url` | URL trang đích. |
| `products[]` | Mảng sản phẩm: `product_name`, `price`, `currency`, `sku` (như bảng [1.4](#14-products--sản-phẩm-bóc-từ-trang-đích-của-quảng-cáo)). |
| `error` | Thông báo lỗi nếu crawl/AI thất bại. |

### 5.4. `page_<tên>_<stamp>.md`

Nội dung trang đích đã được crawl4ai chuyển sang Markdown — đây là đầu vào AI dùng để bóc sản phẩm. Không có cấu trúc cột.

### Quy ước tên file trong `exports/live/`

`<loại>_<tham số>_<YYYYMMDDTHHMMSSZ>.<ext>` — `stamp` là thời điểm lưu (UTC). `export_live.py` luôn lấy file mới nhất theo stamp (riêng `products_extracted_*` thì gộp mọi lượt, lượt sau đè lượt trước theo URL).
