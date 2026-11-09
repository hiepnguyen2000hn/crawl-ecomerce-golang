# Allegro Product Comments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lấy tối đa 5 comment (opinie) mỗi sản phẩm Allegro bằng DOM scraping và lưu vào bảng `allegro_comments`.

**Architecture:** `fetch.py` sau khi bóc listing sẽ dùng chính browser context đã vượt DataDome để mở từng `product_url`, bóc review từ DOM, sort theo lượt helpful lấy top 5, gắn vào offer JSON. Go consumer đọc field `comments` của mỗi offer và INSERT vào `allegro_comments` (FK tới `allegro_raw`). Bật mặc định; best-effort (lỗi comment không fail job).

**Tech Stack:** Go (go-zero), Python (Camoufox/Playwright sync API), PostgreSQL, RabbitMQ.

**Spec:** `docs/superpowers/specs/2026-11-09-allegro-product-comments-design.md`

## Global Constraints

- Migration mới phải là **000013** (`000012_allegro` đã tồn tại; deploy đã có 000011_fbads_media).
- Comment là **best-effort**: mọi lỗi khi crawl/parse comment phải bị nuốt (log stderr) và KHÔNG làm fail job hay thay đổi exit code của `fetch.py`.
- Mặc định BẬT comment khi `JobMessage.with_comments` vắng mặt; `max_comments <= 0` → **5**.
- Không đổi hành vi listing hiện có; các exit code (0/2/3/4) giữ nguyên ý nghĩa.
- Bảng `allegro_comments` tham chiếu `allegro_raw(id)` với `ON DELETE CASCADE`.

## Review Focus

- **Offer không có `url`** (product_id/link null): phải bỏ qua, không goto, không crash → pinned trong Task 4 (`select_top_comments`/loop guard) và Task 3 (insert bỏ qua offer rỗng comment).
- **Product page bị DataDome chặn/timeout**: extractor trả `[]`, job vẫn hoàn tất với listing → pinned Task 4 (try/except quanh extractor, test ở mức hàm pure không bao phủ browser nên ghi chú manual spike).
- **Comment nhiều hơn 5 / thiếu helpful_count**: sort ổn định, `helpful_count` null coi như 0, cắt đúng 5 → pinned Task 4 `select_top_comments`.
- **`with_comments` vắng mặt trong message cũ**: phải default true, MaxComments default 5 → pinned Task 3 test.
- **api-gateway gửi thêm field nhưng consumer/listing-only client cũ**: field optional, không vỡ unmarshal → pinned Task 3 (JobMessage dùng pointer/omitempty) + Task 5.

---

### Task 1: Migration `allegro_comments`

**Files:**
- Create: `db/migrations/000013_allegro_comments.up.sql`
- Create: `db/migrations/000013_allegro_comments.down.sql`

**Interfaces:**
- Produces: bảng `allegro_comments(id, allegro_raw_id, job_id, offer_id, product_id, author, rating, content, review_date, confirmed_purchase, helpful_count, pros, cons, position, raw, fetched_at)`.

- [ ] **Step 1: Viết `000013_allegro_comments.up.sql`** đúng schema trong spec §5 (PK uuid default gen_random_uuid, `allegro_raw_id UUID NOT NULL REFERENCES allegro_raw(id) ON DELETE CASCADE`, `job_id UUID NOT NULL REFERENCES jobs(id)`, `raw JSONB NOT NULL`, 2 index `allegro_comments_raw_id_idx` và `allegro_comments_job_id_idx`).

- [ ] **Step 2: Viết `000013_allegro_comments.down.sql`**: `DROP TABLE IF EXISTS allegro_comments;`

- [ ] **Step 3: Verify áp được migration** (nếu có DB local)

Run: `psql "$ALLEGRO_DSN" -f db/migrations/000013_allegro_comments.up.sql`
Expected: `CREATE TABLE` / `CREATE INDEX` không lỗi. (Không có DB → bỏ qua, xác nhận cú pháp bằng mắt.)

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000013_allegro_comments.up.sql db/migrations/000013_allegro_comments.down.sql
git commit -m "feat(allegro): add allegro_comments table (000013)"
```

---

### Task 2: Go — struct `Comment`, `FetchParams`, cờ `command()`

**Files:**
- Modify: `cmd/allegro-service/internal/crawler/crawler.go`
- Test: `cmd/allegro-service/internal/crawler/crawler_test.go`

**Interfaces:**
- Produces:
  - `type Comment struct { Author *string; Rating *float64; Content *string; ReviewDate *string; ConfirmedPurchase bool; HelpfulCount *int; Pros *string; Cons *string; Position int }` với JSON tags `author,rating,content,review_date,confirmed_purchase,helpful_count,pros,cons,position`.
  - Field mới trong `Offer`: `Comments []Comment \`json:"comments"\``.
  - Field mới trong `FetchParams`: `WithComments bool`, `MaxComments int`.
  - `command()` thêm `--with-comments` khi `WithComments`, và `--max-comments <n>` khi `MaxComments > 0`.

- [ ] **Step 1: Thêm test `TestCommandBuilderWithComments`** trong `crawler_test.go`

```go
func TestCommandBuilderWithComments(t *testing.T) {
    s := NewSubprocess(Config{Python: "/venv/bin/python", Script: "fetch.py"})
    _, args := s.command(FetchParams{
        URL: "https://allegro.pl/listing?string=laptop",
        WithComments: true, MaxComments: 5,
    })
    // phải chứa "--with-comments" và "--max-comments","5" theo đúng thứ tự liền nhau
}
```
Assertions: `args` chứa `"--with-comments"`; và chứa cặp liền nhau `"--max-comments", "5"`. (Dùng helper so sánh subslice hoặc kiểm tra index.)

- [ ] **Step 2: Run test — fail**

Run: `go test ./cmd/allegro-service/internal/crawler/ -run TestCommandBuilderWithComments -v`
Expected: FAIL (field `WithComments` chưa tồn tại / cờ chưa sinh).

- [ ] **Step 3: Implement** struct `Comment`, field `Offer.Comments`, `FetchParams.WithComments/MaxComments`, và nhánh cờ trong `command()` (đặt sau nhánh `--max-items`, trước `--profile`).

- [ ] **Step 4: Run test — pass** (và cả `TestCommandBuilder` cũ vẫn pass)

Run: `go test ./cmd/allegro-service/internal/crawler/ -v`
Expected: PASS all.

- [ ] **Step 5: Commit**

```bash
git add cmd/allegro-service/internal/crawler/crawler.go cmd/allegro-service/internal/crawler/crawler_test.go
git commit -m "feat(allegro): crawler Comment struct + --with-comments/--max-comments flags"
```

---

### Task 3: Go — consumer wiring + insert comments

**Files:**
- Modify: `cmd/allegro-service/internal/worker/consumer.go`
- Test: `cmd/allegro-service/internal/worker/consumer_test.go`

**Interfaces:**
- Consumes: Task 2 `crawler.FetchParams{WithComments,MaxComments}`, `crawler.Offer.Comments`.
- Produces:
  - `JobMessage` thêm `WithComments *bool \`json:"with_comments,omitempty"\``, `MaxComments int \`json:"max_comments,omitempty"\``.
  - helper `func commentsEnabled(msg JobMessage) bool` (nil → true) và `func commentsLimit(msg JobMessage) int` (<=0 → 5).

- [ ] **Step 1: Thêm test** `TestCommentsDefaults` trong `consumer_test.go`

```go
func TestCommentsDefaults(t *testing.T) {
    // with_comments vắng mặt → enabled=true, limit=5
    var m JobMessage
    _ = json.Unmarshal([]byte(`{"job_id":"1"}`), &m)
    if !commentsEnabled(m) { t.Fatal("default should enable comments") }
    if commentsLimit(m) != 5 { t.Fatalf("default limit want 5 got %d", commentsLimit(m)) }
    // with_comments=false → disabled
    var m2 JobMessage
    _ = json.Unmarshal([]byte(`{"job_id":"1","with_comments":false,"max_comments":3}`), &m2)
    if commentsEnabled(m2) { t.Fatal("explicit false should disable") }
    if commentsLimit(m2) != 3 { t.Fatal("explicit limit 3") }
}
```

- [ ] **Step 2: Run test — fail**

Run: `go test ./cmd/allegro-service/internal/worker/ -run TestCommentsDefaults -v`
Expected: FAIL (helpers/fields chưa có).

- [ ] **Step 3: Implement** fields `JobMessage.WithComments/MaxComments`, `commentsEnabled`, `commentsLimit`; truyền `WithComments: commentsEnabled(msg)`, `MaxComments: commentsLimit(msg)` vào `crawler.FetchParams` trong vòng `HandleMessage`. Sửa INSERT `allegro_raw` thành `... RETURNING id` (dùng `QueryRowContext(...).Scan(&allegroRawID)`); sau đó với mỗi `o.Comments` chạy INSERT vào `allegro_comments(allegro_raw_id, job_id, offer_id, product_id, author, rating, content, review_date, confirmed_purchase, helpful_count, pros, cons, position, raw)`. Lỗi insert comment chỉ log (`fmt.Println`), **không** `markFailed`/return.

- [ ] **Step 4: Run test — pass** (cả `TestResolveTargets`, `TestHandleMessage_*` cũ vẫn pass)

Run: `go test ./cmd/allegro-service/internal/worker/ -v`
Expected: PASS all.

- [ ] **Step 5: Commit**

```bash
git add cmd/allegro-service/internal/worker/consumer.go cmd/allegro-service/internal/worker/consumer_test.go
git commit -m "feat(allegro): persist product comments into allegro_comments"
```

---

### Task 4: Python — DOM comment extraction trong `fetch.py`

**Files:**
- Modify: `cmd/allegro-service/crawler/fetch.py`
- Create: `cmd/allegro-service/crawler/fetch_comments_test.py` (test hàm pure)

**Interfaces:**
- Consumes: `argv` `--with-comments`, `--max-comments` (Task 2 sinh ra).
- Produces: mỗi offer trong JSON stdout có thêm `"comments": [ {author,rating,content,review_date,confirmed_purchase,helpful_count,pros,cons,position}, ... ]`.
- Hàm pure test được: `select_top_comments(comments: list[dict], max_n: int) -> list[dict]` — sort theo `helpful_count` (None→0) giảm dần, ổn định, cắt `max_n`, gán `position` = 1..n.

- [ ] **Step 1: Viết test** `fetch_comments_test.py` cho `select_top_comments`

```python
def test_select_top_comments_sorts_by_helpful_and_caps():
    raw = [
        {"content":"a","helpful_count":1},
        {"content":"b","helpful_count":9},
        {"content":"c","helpful_count":None},
        {"content":"d","helpful_count":5},
        {"content":"e","helpful_count":3},
        {"content":"f","helpful_count":7},
    ]
    got = select_top_comments(raw, 5)
    assert [c["content"] for c in got] == ["b","f","d","e","a"]
    assert [c["position"] for c in got] == [1,2,3,4,5]
```

- [ ] **Step 2: Run test — fail**

Run: `python -m pytest cmd/allegro-service/crawler/fetch_comments_test.py -q` (hoặc `.venv/bin/python`)
Expected: FAIL (`select_top_comments` chưa có / import lỗi). Lưu ý: đặt `select_top_comments` sao cho import được mà không khởi tạo Camoufox (import nặng nằm trong `main`/guard nếu cần).

- [ ] **Step 3: Implement**
  - `select_top_comments(comments, max_n)` thuần Python.
  - `COMMENTS_JS` (JS string) bóc mọi review trong section opinie: `author`, `rating` (parse sao), `content`, `review_date`, `confirmed_purchase` (có chuỗi "potwierdzona zakupem"), `helpful_count` (số lượt "pomocna"/vote), `pros`, `cons`. Viết theo kiểu resilient (querySelectorAll + regex), giống `EXTRACT_JS`.
  - `extract_comments_from_product(page, product_url, max_comments)`: `goto` → `settle_or_solve` → `lazy_scroll` → (click "pokaż więcej/kolejne opinie" tối đa vài lần nếu < max_comments) → `page.evaluate(COMMENTS_JS)` → `select_top_comments`. Toàn bộ bọc `try/except` trả `[]` khi lỗi.
  - CLI: thêm `--with-comments` (`action="store_true"`), `--max-comments` (`type=int, default=5`).
  - Trong `main()` sau khi có `all_offers`: nếu `args.with_comments`, lặp offer có `o["url"]`, gọi extractor, gán `o["comments"]`; offer không có url → `o["comments"] = []`.

- [ ] **Step 4: Run test — pass**

Run: `python -m pytest cmd/allegro-service/crawler/fetch_comments_test.py -q`
Expected: PASS.

- [ ] **Step 5: Spike xác minh DOM (manual, bắt buộc trước khi coi Task xong)**

Run: `./scripts/camoufox-yolo-captcha/.venv/bin/python cmd/allegro-service/crawler/fetch.py --keyword "laptop" --pages 1 --max-items 2 --with-comments --max-comments 5`
Expected: JSON stdout, mỗi offer có mảng `comments` (≥1 nếu sản phẩm có opinie). Nếu rỗng toàn bộ → mở 1 product URL thủ công, chỉnh selector `COMMENTS_JS` cho khớp DOM thật rồi chạy lại. Ghi lại selector thật vào comment trong code.

- [ ] **Step 6: Commit**

```bash
git add cmd/allegro-service/crawler/fetch.py cmd/allegro-service/crawler/fetch_comments_test.py
git commit -m "feat(allegro): scrape top-5 product comments from DOM"
```

---

### Task 5: api-gateway — expose `with_comments` / `max_comments`

**Files:**
- Modify: `api/gateway.api` (type `CreateAllegroJobRequest`)
- Modify: `cmd/api-gateway/internal/types/types.go` (struct sinh sẵn — sửa tay cho khớp, không cần goctl)
- Modify: `cmd/api-gateway/internal/logic/createallegrojoblogic.go`

**Interfaces:**
- Consumes: `JobMessage.with_comments/max_comments` (Task 3).
- Produces: POST `/jobs/allegro` nhận thêm `with_comments` (bool, optional), `max_comments` (int, optional) và publish kèm.

- [ ] **Step 1: Thêm field vào `api/gateway.api`** `CreateAllegroJobRequest`: `WithComments *bool \`json:"with_comments,optional"\``, `MaxComments int \`json:"max_comments,optional"\``.

- [ ] **Step 2: Đồng bộ `types.go`** thêm 2 field tương ứng (`WithComments *bool`, `MaxComments int`).

- [ ] **Step 3: Sửa publish** trong `createallegrojoblogic.go`: thêm `"with_comments": req.WithComments`, `"max_comments": req.MaxComments` vào map `msg`.

- [ ] **Step 4: Verify build toàn repo**

Run: `go build ./...`
Expected: không lỗi.

- [ ] **Step 5: Commit**

```bash
git add api/gateway.api cmd/api-gateway/internal/types/types.go cmd/api-gateway/internal/logic/createallegrojoblogic.go
git commit -m "feat(api-gateway): allegro job with_comments/max_comments options"
```

---

### Task 6: Cập nhật README + chạy toàn bộ test

**Files:**
- Modify: `cmd/allegro-service/README.md`

- [ ] **Step 1: Cập nhật README**: thêm mô tả bước crawl comment vào sơ đồ flow §1, thêm bảng `allegro_comments` vào §3, và ví dụ body có `"with_comments": true, "max_comments": 5` ở §4.1.

- [ ] **Step 2: Verify toàn bộ**

Run: `go build ./... && go test ./cmd/allegro-service/...`
Expected: build OK, test PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/allegro-service/README.md
git commit -m "docs(allegro): document product comments flow + allegro_comments"
```
