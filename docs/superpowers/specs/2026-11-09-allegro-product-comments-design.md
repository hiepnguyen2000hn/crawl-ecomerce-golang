# Thiết kế: Lấy comment (opinie) từng sản phẩm Allegro bằng DOM scraping

Ngày: 2026-11-09
Trạng thái: chờ duyệt

## 1. Ý định & lý do

`allegro-service` hiện chỉ crawl trang **listing** (các `<article>` card trong kết
quả tìm kiếm). Mỗi offer lấy được `rating` + `reviews_count` (số sao và số lượng
đánh giá) nhưng **không có nội dung comment**. Người dùng muốn lấy thêm **comment
thực tế của từng sản phẩm** để phục vụ phân tích (ai-service).

Cách tiếp cận đã chốt: **scrape DOM** — sau khi có offer, mở `product_url`, cuộn tới
section "Opinie", bóc từng review từ DOM.

## 2. Goals / Non-goals / Anti-goals

**Goals**
- Mỗi sản phẩm trong job lấy tối đa **5 comment**, ưu tiên comment nhiều lượt
  "pomocna" (helpful/vote) nhất.
- Mỗi comment lưu: tác giả, số sao, ngày, nội dung, có "Opinia potwierdzona
  zakupem" (xác nhận mua) hay không, số lượt helpful, pros/cons nếu có.
- Lưu vào bảng riêng `allegro_comments`, quan hệ 1-nhiều với `allegro_raw`.
- Không phá vỡ hành vi job hiện tại (listing vẫn chạy như cũ).

**Non-goals**
- Không phân trang hết toàn bộ comment của mỗi sản phẩm (chỉ ~5).
- Không gọi API opinie nội bộ của Allegro (chủ ý dùng DOM theo yêu cầu).
- Không thay đổi ai-service trong phạm vi này (chỉ ghi dữ liệu; tích hợp AT là
  việc sau).

**Anti-goals**
- Không để việc crawl comment làm job bị DataDome chặn hoặc vượt `TimeoutSeconds`
  đến mức listing (phần dữ liệu chính) bị mất. Comment là **best-effort**: lỗi khi
  lấy comment không được làm fail cả job.

## 3. Ràng buộc

- Mỗi `page.goto(product_url)` là một request mới qua DataDome → rủi ro bị
  challenge/captcha. Phải tái dùng `settle_or_solve` và bọc try/except, bỏ qua
  sản phẩm lỗi thay vì dừng.
- `TimeoutSeconds` (mặc định 300s, config `Allegro.TimeoutSeconds`) bao trùm cả
  subprocess. Thêm N product page sẽ tăng thời gian → cần tính tới việc giới hạn
  và/hoặc người dùng tăng timeout khi bật comment.
- DOM section opinie của Allegro render bằng JS, có thể cần cuộn và/hoặc click
  "Pokaż więcej opinii"; cấu trúc selector chưa xác nhận 100% → EXTRACT viết theo
  kiểu heuristic/resilient giống `EXTRACT_JS` hiện có, và **xác minh trên 1 trang
  sản phẩm thật ngay bước đầu khi implement** (spike sớm).

## 4. Các hướng đã cân nhắc

Yêu cầu đã chốt là DOM scraping, nên trục chính là cách tổ chức việc visit:

1. **Visit tuần tự trong cùng vòng lặp page của fetch.py (CHỌN).** Sau khi bóc
   offers của một trang listing, với mỗi offer có `product_url`, dùng chính `page`
   (hoặc page mới trong cùng browser context đã vượt DataDome) để goto, bóc
   comment, gắn vào offer. Tái dùng session/cookies đã "ấm" → ít bị chặn hơn.
   Đơn giản, không cần đổi kiến trúc Go.
2. Tách thành subprocess/endpoint riêng cho comment. Linh hoạt hơn nhưng phải
   dựng thêm job type / queue, và mất session đã vượt DataDome của listing → dễ bị
   chặn hơn, phức tạp hơn nhiều. Loại.

→ Chọn hướng 1.

## 5. Các quyết định thiết kế (đã chốt)

**Dữ liệu — bảng mới (migration `000013_allegro_comments`)**

```sql
CREATE TABLE IF NOT EXISTS allegro_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    allegro_raw_id UUID NOT NULL REFERENCES allegro_raw(id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES jobs(id),
    offer_id TEXT,
    product_id TEXT,
    author TEXT,
    rating NUMERIC,
    content TEXT,
    review_date TEXT,
    confirmed_purchase BOOLEAN,
    helpful_count INTEGER,
    pros TEXT,
    cons TEXT,
    position INTEGER,          -- thứ tự comment trong 5 cái lấy được
    raw JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS allegro_comments_raw_id_idx ON allegro_comments (allegro_raw_id);
CREATE INDEX IF NOT EXISTS allegro_comments_job_id_idx ON allegro_comments (job_id);
```
(`000013_allegro_comments.down.sql` drop bảng.)

> Lưu ý số migration: memory ghi nhận deploy đã có 000011_fbads_media và allegro
> phải là 000012 — đã đúng (000012_allegro tồn tại). Comment dùng **000013**.

**fetch.py**
- Thêm CLI flags: `--with-comments` (mặc định tắt ở tầng CLI) và `--max-comments N`
  (mặc định 5).
- Hàm `extract_comments_from_product(page, max_comments)`:
  - goto `product_url`, `settle_or_solve`, cuộn tới section opinie, (click "xem
    thêm" nếu cần ít lần), `page.evaluate(COMMENTS_JS)`.
  - `COMMENTS_JS` bóc mọi review thấy được: author, rating, date, content,
    confirmed_purchase, helpful_count, pros, cons.
  - Sort theo `helpful_count` giảm dần, lấy **top 5**.
  - Bọc try/except: lỗi → trả `[]`, in cảnh báo ra stderr, không làm fail job.
- Trong `main()`, sau khi có `all_offers`, nếu bật comment thì lặp qua từng offer
  có `url`, gọi extractor, gắn `o["comments"] = [...]`.
- JSON stdout: mỗi offer có thêm field `comments`.

**Go — crawler.go**
- Struct `Comment` + field `Comments []Comment \`json:"comments"\`` trong `Offer`.
- `FetchParams` thêm `WithComments bool`, `MaxComments int`; `command()` thêm cờ
  `--with-comments` / `--max-comments`.

**Go — consumer.go**
- `JobMessage` thêm `WithComments *bool \`json:"with_comments,omitempty"\`` và
  `MaxComments int \`json:"max_comments,omitempty"\``.
- **Mặc định bật comment** khi field vắng (vì đây là mục tiêu người dùng muốn
  ngay): nếu `WithComments == nil` → coi là true; `MaxComments<=0` → 5.
- INSERT `allegro_raw ... RETURNING id`, lấy `allegroRawID`, rồi loop INSERT
  `allegro_comments` cho từng comment của offer đó.

**Go — api-gateway**
- `CreateAllegroJobRequest` thêm `WithComments *bool`, `MaxComments int`; publish
  kèm vào message. (Không bắt buộc để chạy được — consumer đã default-on — nhưng
  thêm để client bật/tắt được.)

**Scope sản phẩm**: tất cả offer mà job crawl (đã bị giới hạn bởi `max_items`).
Không thêm giới hạn số sản phẩm riêng trong vòng lặp này. (Có thể thêm flag sau
nếu thực tế bị chặn/timeout.)

## 6. Phần để lại cho người implement

- Xác nhận selector thật của section opinie Allegro ngay bước đầu (spike trên 1
  product URL), rồi hoàn thiện `COMMENTS_JS` theo kiểu resilient.
- Quyết định chi tiết cách phát hiện/limit click "xem thêm opinie" (đủ để có ≥5
  comment là dừng).
- Test: `consumer_test.go` và `crawler_test.go` đã có — thêm case cho mapping
  comment và cho `command()` sinh đúng cờ `--with-comments/--max-comments`.
- Cập nhật README allegro-service mô tả flow mới + bảng `allegro_comments`.
```
```
