# fb-ads media download Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ngay sau khi crawl, tải các ảnh SD của FB ads (ảnh quảng cáo và ảnh bìa video) về ổ đĩa local trên VPS. Lý do: link fbcdn có chữ ký, hết hạn sau vài ngày theo tham số `oe=`.

**Architecture:** `fb-ads-service` trích các URL media từ `fbads_raw.raw` và upsert vào bảng mới `fbads_media` với trạng thái `pending`. Một consumer thứ hai trong cùng service lắng nghe sự kiện `crawl.completed.fbads` đang có sẵn (qua queue riêng `fbads.media.completed`), tải file vào một thư mục trên host được bind mount vào container, rồi ghi lại kết quả. Trên VPS, nginx phục vụ thẳng thư mục đó tại `/media/` (không đi qua service Go nào). Cột `fbads_raw.raw` không bao giờ bị sửa.

**Tech Stack:** Go 1.25, Postgres (golang-migrate), RabbitMQ (`pkg/rabbitmq`), Docker Compose bind mount, nginx trên VPS.

**Spec:** Quyết định chốt trong cuộc trao đổi ngày 2026-11-03 (không có tài liệu spec riêng):
- Lưu trên **ổ đĩa local** của VPS. Không dùng S3/MinIO, không lưu bytes trong Postgres. (Đã cân nhắc MinIO: image Docker chính thức của MinIO không còn pull được, community edition đã ngừng phát hành.)
- Trên VPS, nginx phục vụ ảnh tại `/media/`.
- **Chỉ ảnh SD:** `resized_image_url` và **ảnh bìa video** `video_preview_image_url`. **Không tải video**, không lấy HD/original (trừ trường hợp fallback ghi bên dưới), không lấy ảnh page/profile.
- **Không ghi đè JSON `raw`.** Media được theo dõi trong bảng riêng.
- Từ mẫu thật: link hết hạn khoảng 4–5 ngày sau khi crawl. `oe=` là unix seconds viết dạng hex (ví dụ `6AE8D26D` = 2026-11-02T18:11:57Z).

## Global Constraints

- Module `github.com/hiepnv/crawl-ecomerce-golang`. Dockerfile dùng `golang:1.25-alpine`.
- Exchange `crawl`. Routing key nguồn: `crawl.completed.fbads` (đã được publish ở `cmd/fb-ads-service/internal/worker/consumer.go`). Tên queue mới: `fbads.media.completed`.
- Migration mới là `000011_fbads_media`.
- Thư mục media trong container: `/data/media`. Trên host dùng **bind mount** `${MEDIA_HOST_DIR:-./data/media}` (trên VPS đặt `MEDIA_HOST_DIR=/srv/crawl/media` trong `.env`), chỉ gắn vào `fb-ads-service`, quyền rw. Nginx trên host đọc thẳng `MEDIA_HOST_DIR`. Không dùng named volume, để `docker compose down -v` không xoá mất ảnh.
- Storage key là đường dẫn tương đối dưới thư mục media: `fbads/<ad_archive_id>/<slug>.<ext>`, trong đó slug là json path đã đổi `.`/`[`/`]` thành `_` và gộp các `_` liên tiếp. Ví dụ: `snapshot.images[0].resized_image_url` → `fbads/2179652559429754/snapshot_images_0_resized_image_url.jpg`.
- Test unit không bao giờ gọi fbcdn thật. Dùng `httptest.Server`. Test có dùng DB thì bỏ qua khi `FBADS_MEDIA_TEST_DSN` không được set (theo cách làm của `pkg/apify/slots_test.go`).
- Lỗi khi tải media **không bao giờ** được làm job crawl thất bại, cũng không được làm chạy lại Apify.

## Review Focus

1. **Một ad bị crawl lại ở job sau** mang URL mới đã ký lại cho cùng json path. Nếu dòng cũ đang `done` thì giữ nguyên; nếu đang `failed`/`expired` thì quay về `pending` với URL mới. Test: `TestUpsertPending_RevivesFailedKeepsDone` (Task 2).
2. **fbcdn trả 403 khi link đã hết hạn** thì đánh `expired` ngay, không retry. Test: `TestDownloader_403MarksExpired` (Task 3).
3. **Server trả về trang HTML lỗi với status 200 hoặc file quá lớn** thì không được lưu thành `.jpg`. Test: `TestDownloader_RejectsNonImage`, `TestDownloader_RejectsOversize` (Task 3).
4. **`resized_image_url` là `""` hoặc `null`** thì không sinh ra ref rỗng. Nếu ad có `original_image_url` thì fallback sang link đó. Test: `TestExtract_EmptyResizedFallsBackToOriginal` (Task 1).
5. **Container ghi file với uid root, nginx (user `www-data`) phải đọc được.** File được tạo với quyền 0644, thư mục 0755. Test: `TestDownloader_SavesJPEG` kiểm tra mode của file (Task 3). Khi deploy thì kiểm tra lại bằng `curl` (Task 5).

---

## File Structure

```
cmd/fb-ads-service/internal/media/
  extract.go            # ExtractRefs, ParseExpiry, StorageKey (pure, no I/O)
  extract_test.go
  store.go              # fbads_media SQL: UpsertPending, PendingForJob, MarkDone, MarkFailed
  store_test.go         # DSN-gated
  downloader.go         # HTTP fetch → validated file on disk
  downloader_test.go
  worker.go             # crawl.completed.fbads handler: PendingForJob → Downloader → Mark*
  worker_test.go
  testdata/apify_ads_sample.json   # ĐÃ TẠO SẴN — 4 ad thật (bản rút gọn)
db/migrations/000011_fbads_media.{up,down}.sql
cmd/fb-ads-service/internal/config/config.go      # + Media block
cmd/fb-ads-service/internal/worker/consumer.go    # + upsert pending refs
cmd/fb-ads-service/main.go                        # + second consumer
cmd/fb-ads-service/etc/fbads{,.docker}.yaml       # + Media.Dir
docker-compose.yml                                # bind mount ${MEDIA_HOST_DIR}
.env.example, .gitignore                          # MEDIA_HOST_DIR, data/
deploy/nginx/fbads-media.conf                     # location /media/ snippet
docs/deploy/fbads-media.md                        # các bước deploy lên VPS
```

---

### Task 1: Trích media ref (hàm thuần)

**Files:**
- Create: `cmd/fb-ads-service/internal/media/extract.go`, `extract_test.go`
- Fixture (đã có sẵn): `cmd/fb-ads-service/internal/media/testdata/apify_ads_sample.json`

**Interfaces:**
- Produces:
  ```go
  type Kind string
  const (KindImage Kind = "image"; KindVideoCover Kind = "video_cover")
  type Ref struct { JSONPath string; Kind Kind; URL string; ExpiresAt time.Time } // ExpiresAt zero = unknown
  func ExtractRefs(raw json.RawMessage) ([]Ref, error)
  func ParseExpiry(rawURL string) time.Time
  func StorageKey(adArchiveID, jsonPath, ext string) string
  ```

Quy tắc trích, áp dụng cho `snapshot.images`, `snapshot.extra_images`, `snapshot.cards`, `snapshot.videos`, `snapshot.extra_videos` theo thứ tự đó, giữ nguyên index:
- Phần tử có `resized_image_url` → `KindImage`. Nếu field này rỗng hoặc null thì fallback sang `original_image_url` (JSONPath ghi tên field thực sự được dùng).
- Phần tử có `video_preview_image_url` khác rỗng → `KindVideoCover`.
- Một phần tử trong `cards` có thể cho ra cả hai loại.
- Không đọc bất kỳ field nào khác (`video_*_url`, `watermarked_*`, `page_profile_picture_url`, `_details.*`).

- [ ] **Step 1: Viết test fail**

```go
func TestExtract_RealSample(t *testing.T) // load fixture, ExtractRefs từng phần tử, gộp lại
//  total == 4
//  ad 1083605804695644 → [{JSONPath:"snapshot.videos[0].video_preview_image_url", Kind:KindVideoCover}]
//  ad 2179652559429754 → [{JSONPath:"snapshot.images[0].resized_image_url", Kind:KindImage}],
//     URL bắt đầu bằng "https://scontent.fakl5-2.fna.fbcdn.net/v/t39.35426-6/864828879_"
//  không URL nào chứa ".mp4", "s60x60" hoặc "t39.30808"
func TestExtract_EmptyResizedFallsBackToOriginal(t *testing.T)
//  {"snapshot":{"images":[{"resized_image_url":"","original_image_url":"https://x/o.jpg?oe=6AE8D26D"}]}}
//   → 1 ref, JSONPath "snapshot.images[0].original_image_url"
//  {"snapshot":{"images":[{"resized_image_url":null}]}} → 0 refs, no error
func TestExtract_CardsYieldBoth(t *testing.T)
//  cards[0] có cả resized_image_url và video_preview_image_url → 2 refs, paths snapshot.cards[0].…
func TestParseExpiry(t *testing.T)
//  "https://x/a.jpg?oe=6AE8D26D" → time.Date(2026,11,2,18,11,57,0,time.UTC)
//  thiếu oe / "oe=zz" → time.Time{}
func TestStorageKey(t *testing.T)
//  StorageKey("2179652559429754","snapshot.images[0].resized_image_url",".jpg")
//   == "fbads/2179652559429754/snapshot_images_0_resized_image_url.jpg"
//  StorageKey("../x","a",".jpg") phải KHÔNG chứa ".." (ad id được làm sạch: chỉ giữ [0-9A-Za-z_-])
```

- [ ] **Step 2:** `go test ./cmd/fb-ads-service/internal/media/ -run 'Extract|ParseExpiry|StorageKey'`. Kỳ vọng: FAIL (undefined).
- [ ] **Step 3:** Viết `extract.go`. Decode vào struct gồm `snapshot` với các slice phần tử dùng `*string` cho các field URL, để rỗng và null được xử lý giống nhau. `ParseExpiry` dùng `url.Parse` + `strconv.ParseInt(oe,16,64)`.
- [ ] **Step 4:** Chạy lại lệnh trên. Kỳ vọng: PASS.
- [ ] **Step 5:** Commit `feat(fbads): extract SD image and video-cover refs from Apify snapshot`.

---

### Task 2: Bảng `fbads_media` và store

**Files:**
- Create: `db/migrations/000011_fbads_media.up.sql`, `.down.sql`
- Create: `cmd/fb-ads-service/internal/media/store.go`, `store_test.go`

**Interfaces:**
- Consumes: `Ref`, `StorageKey` (Task 1)
- Produces:
  ```go
  type Row struct { ID, AdArchiveID, JSONPath string; Kind Kind; URL string; ExpiresAt time.Time }
  type Store struct{ DB *sql.DB }
  func (s *Store) UpsertPending(ctx context.Context, adArchiveID string, refs []Ref) error
  func (s *Store) PendingForJob(ctx context.Context, jobID string) ([]Row, error)
  func (s *Store) MarkDone(ctx context.Context, id, storageKey, contentType string, size int64) error
  func (s *Store) MarkFailed(ctx context.Context, id string, expired bool, errMsg string) error
  ```

Schema (up):
```sql
CREATE TABLE fbads_media (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ad_archive_id TEXT NOT NULL,
  json_path TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('image','video_cover')),
  source_url TEXT NOT NULL,
  expires_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','done','failed','expired')),
  storage_key TEXT, content_type TEXT, size_bytes BIGINT,
  attempts INT NOT NULL DEFAULT 0, error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (ad_archive_id, json_path)
);
CREATE INDEX fbads_media_pending_idx ON fbads_media (ad_archive_id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS fbads_raw_job_id_idx ON fbads_raw (job_id);
```
Down: drop bảng, drop `fbads_raw_job_id_idx`.

Hành vi `UpsertPending`: `ON CONFLICT (ad_archive_id, json_path) DO UPDATE SET source_url, expires_at, status='pending', attempts=0, error=NULL, updated_at=now() WHERE fbads_media.status <> 'done'`. Chạy hết mọi ref trong một transaction. `expires_at` zero thì ghi NULL.
`PendingForJob`: các dòng `status='pending'` có `ad_archive_id IN (SELECT ad_archive_id FROM fbads_raw WHERE job_id=$1)`.
`MarkFailed`: `attempts = attempts+1`, status = `expired` nếu `expired` là true, ngược lại là `failed`.

- [ ] **Step 1: Viết test fail** (bỏ qua khi `FBADS_MEDIA_TEST_DSN` không set; mỗi test tự truncate `fbads_media`)

```go
func TestUpsertPending_Idempotent(t *testing.T)            // upsert 2 lần cùng refs → 1 dòng mỗi path
func TestUpsertPending_RevivesFailedKeepsDone(t *testing.T)
//  path A → MarkDone; path B → MarkFailed(expired=true)
//  upsert lại với URL mới cho cả hai → A vẫn 'done' giữ URL cũ; B 'pending', URL mới, attempts 0
func TestPendingForJob_ScopedToJob(t *testing.T)           // ad của job khác không xuất hiện
```

- [ ] **Step 2:** Áp migration lên DB test, chạy `FBADS_MEDIA_TEST_DSN=... go test ./cmd/fb-ads-service/internal/media/ -run 'Upsert|Pending'`. Kỳ vọng: FAIL.
- [ ] **Step 3:** Viết migration và `store.go`.
- [ ] **Step 4:** Chạy lại. Kỳ vọng: PASS. Chạy `migrate ... down 1 && up` để kiểm tra down migration chạy sạch.
- [ ] **Step 5:** Commit `feat(fbads): add fbads_media table and store`.

---

### Task 3: Downloader

**Files:** Create `cmd/fb-ads-service/internal/media/downloader.go`, `downloader_test.go`

**Interfaces:**
- Consumes: `Row`, `StorageKey`
- Produces:
  ```go
  type Downloader struct { HTTP *http.Client; Dir string; MaxBytes int64 }
  type Result struct { StorageKey, ContentType string; Size int64 }
  var ErrExpired = errors.New("media: source url expired")
  func (d *Downloader) Fetch(ctx context.Context, r Row) (Result, error)
  ```

Hành vi:
- GET kèm User-Agent của trình duyệt.
- Status 403/404/410 → `ErrExpired`. Các status khác 2xx → lỗi thường.
- `Content-Type` phải bắt đầu bằng `image/`. Đuôi file lấy từ content type: `image/jpeg`→`.jpg`, `image/png`→`.png`, `image/webp`→`.webp`, còn lại→`.img`.
- Đọc qua `io.LimitReader(MaxBytes+1)`, vượt quá thì báo lỗi.
- Ghi vào `<Dir>/<key>.tmp`, xong mới `os.Rename` thành `<Dir>/<key>`. Thư mục cha tạo với quyền 0755, file chmod 0644 (để nginx đọc được).

- [ ] **Step 1: Viết test fail** (dùng `httptest.Server` + `t.TempDir()`)

```go
func TestDownloader_SavesJPEG(t *testing.T)       // 200 image/jpeg "abc" → file Dir/fbads/<id>/…_url.jpg == "abc", Size 3, mode&0o777 == 0o644
func TestDownloader_403MarksExpired(t *testing.T) // errors.Is(err, ErrExpired)
func TestDownloader_RejectsNonImage(t *testing.T) // 200 text/html → err, không có file nào trong Dir
func TestDownloader_RejectsOversize(t *testing.T) // MaxBytes 10, body 11 byte → err, không có file .jpg và .tmp
```

- [ ] **Step 2:** `go test ./cmd/fb-ads-service/internal/media/ -run Downloader`. Kỳ vọng: FAIL.
- [ ] **Step 3:** Viết `downloader.go`. Khi lỗi giữa chừng thì xoá file `.tmp`.
- [ ] **Step 4:** Chạy lại. Kỳ vọng: PASS.
- [ ] **Step 5:** Commit `feat(fbads): download media to local disk with type and size guards`.

---

### Task 4: Nối vào fb-ads-service

**Files:**
- Create: `cmd/fb-ads-service/internal/media/worker.go`, `worker_test.go`
- Modify: `cmd/fb-ads-service/internal/worker/consumer.go` (sau vòng insert `fbads_raw`, trước khi update `jobs`)
- Modify: `cmd/fb-ads-service/internal/config/config.go`, `cmd/fb-ads-service/main.go`, `etc/fbads.yaml`, `etc/fbads.docker.yaml`

**Interfaces:**
- Consumes: Task 1–3
- Produces:
  ```go
  // media package
  type Fetcher interface { Fetch(ctx context.Context, r Row) (Result, error) }
  type Repo interface {
      PendingForJob(ctx context.Context, jobID string) ([]Row, error)
      MarkDone(ctx context.Context, id, storageKey, contentType string, size int64) error
      MarkFailed(ctx context.Context, id string, expired bool, errMsg string) error
  }
  type Worker struct { Repo Repo; Fetcher Fetcher; Concurrency int; Retries int }
  func (w *Worker) HandleMessage(body []byte) error   // body = {"job_id": "..."}
  ```
- Config: `Media struct { Dir string `json:",default=/data/media"`; MaxBytes int64 `json:",default=10485760"`; Concurrency int `json:",default=4"` }`. Trong `fbads.yaml` (chạy local trên host): `Media: { Dir: ./data/media }`. Trong `fbads.docker.yaml`: `Dir: /data/media`.

Hành vi của `Worker.HandleMessage`:
- Context riêng, timeout 10 phút.
- Chạy song song tối đa `Concurrency` file. Mỗi row thử tối đa `Retries` lần (mặc định 3) với backoff 1s/2s/4s.
- `ErrExpired` → `MarkFailed(expired=true)` ngay, không retry. Hết số lần retry → `MarkFailed(false, err.Error())`.
- **Luôn trả về nil.** Lỗi DB ở `PendingForJob` trả về `rabbitmq.ErrRequeue`.
- Log tổng kết: done/expired/failed.

Thay đổi trong consumer: trong vòng insert có sẵn, sau mỗi lần insert gọi `media.ExtractRefs(ad.Raw)`, rồi gọi `MediaStore.UpsertPending(ctx, ad.AdArchiveID, refs)`. Lỗi trích hoặc lỗi upsert **chỉ log**, không bao giờ `markFailed` hay return. Thêm field `Media interface{ UpsertPending(context.Context, string, []media.Ref) error }` vào `Consumer`; nếu nil thì bỏ qua (để các test cũ vẫn chạy).

`main.go`: tạo consumer RabbitMQ thứ hai, `Consume(ctx, "fbads.media.completed", "crawl.completed.fbads", mediaWorker.HandleMessage)`, theo đúng cách làm nhiều consumer ở `cmd/ai-service/main.go:73-90`. Khi khởi động thì `os.MkdirAll(c.Media.Dir, 0755)`.

- [ ] **Step 1: Viết test fail** (`worker_test.go` dùng fake Repo/Fetcher)

```go
func TestWorker_MixedOutcomes(t *testing.T)
//  3 row: fetch OK / ErrExpired / luôn lỗi
//  → MarkDone 1 lần, MarkFailed(expired=true) 1 lần, MarkFailed(false) 1 lần;
//    row luôn lỗi được Fetch đúng Retries lần; ErrExpired chỉ Fetch 1 lần; HandleMessage trả nil
func TestWorker_RepoErrorRequeues(t *testing.T) // PendingForJob lỗi → errors.Is(err, rabbitmq.ErrRequeue)
```
Thêm vào `cmd/fb-ads-service/internal/worker/consumer_test.go`:
```go
func TestHandleMessage_MediaUpsertFailureDoesNotFailJob(t *testing.T)
//  bắt buộc phải có DB thật → bỏ qua khi không có FBADS_MEDIA_TEST_DSN; fake Media trả lỗi → HandleMessage trả nil, jobs.status = 'crawled'
```
Test dùng `Retries: 3` và inject `sleep` là hàm rỗng (field không export `sleep func(time.Duration)`, nil thì dùng `time.Sleep`) để khỏi phải chờ backoff.

- [ ] **Step 2:** `go test ./cmd/fb-ads-service/...`. Kỳ vọng: test mới FAIL.
- [ ] **Step 3:** Viết `worker.go` và sửa consumer/config/main/yaml như mô tả.
- [ ] **Step 4:** `go test ./cmd/fb-ads-service/... && go vet ./cmd/fb-ads-service/...`. Kỳ vọng: PASS.
- [ ] **Step 5:** Commit `feat(fbads): download ad media after each crawl`.

---

### Task 5: Docker compose, nginx và deploy lên VPS

**Files:**
- Modify: `docker-compose.yml`: thêm vào `fb-ads-service` `volumes: ["${MEDIA_HOST_DIR:-./data/media}:/data/media"]`
- Modify: `.env.example` (thêm `MEDIA_HOST_DIR=` kèm comment), `.gitignore` (thêm `/data/`)
- Create: `deploy/nginx/fbads-media.conf` (đoạn cấu hình để `include` trong server block có sẵn):
  ```nginx
  location /media/ {
      alias /srv/crawl/media/;
      autoindex off;
      add_header Cache-Control "public, max-age=2592000, immutable";
      add_header X-Content-Type-Options nosniff;
      types { image/jpeg jpg; image/png png; image/webp webp; }
      default_type application/octet-stream;
      try_files $uri =404;
  }
  ```
- Create: `docs/deploy/fbads-media.md`: các bước deploy ở Step 3.

- [ ] **Step 1:** `docker compose config -q`. Kỳ vọng: không có output. Kiểm tra `docker compose config | grep -A3 'fb-ads-service' | grep /data/media` ra đúng 1 dòng mount.
- [ ] **Step 2: Smoke test ở máy dev** (cần mạng thật): `docker compose up -d --build migrate fb-ads-service`, rồi `POST /jobs/fbads` với `max_items: 5`. Chờ job sang `crawled`, sau đó chạy:
  `docker compose exec postgres psql -U crawl -c "select status,count(*) from fbads_media group by 1"` → có dòng `done`.
  `find data/media/fbads -name '*.jpg' | head` → có file; `file <một file>` → `JPEG image data`.
- [ ] **Step 3: Viết `docs/deploy/fbads-media.md`** với đúng các bước sau:
  1. `ssh vps && cd <repo> && git pull`
  2. `sudo mkdir -p /srv/crawl/media && sudo chmod 755 /srv/crawl /srv/crawl/media`
  3. Thêm `MEDIA_HOST_DIR=/srv/crawl/media` vào `.env`
  4. `docker compose up -d --build migrate fb-ads-service`. Service `migrate` chạy `000011`. Kiểm tra bằng `docker compose logs migrate | tail` → thấy `11/u fbads_media`.
  5. `sudo cp deploy/nginx/fbads-media.conf /etc/nginx/snippets/`, thêm `include snippets/fbads-media.conf;` vào server block, rồi `sudo nginx -t && sudo systemctl reload nginx`
  6. Kiểm tra: `curl -sI https://<domain>/media/<storage_key>` → `200`, `content-type: image/jpeg`; `curl -sI https://<domain>/media/fbads/` → `403` hoặc `404` (không liệt kê thư mục)
  7. Backup: `rsync -a /srv/crawl/media/ <backup-host>:/backup/media/`. Gợi ý cron hằng ngày.
  8. Rollback: `git checkout <commit trước> && docker compose up -d --build fb-ads-service`. Không cần migrate down, vì bảng `fbads_media` không ảnh hưởng tới code cũ.
- [ ] **Step 4:** Commit `chore(deploy): mount media dir and serve it via nginx`.

---

## Ngoài phạm vi (cố ý chưa làm)

- API trả danh sách ad kèm URL media, và field `ad_library_url` (`https://www.facebook.com/ads/library/?id=<ad_archive_id>`). Hiện `GetFbAdsJob` chỉ trả số lượng ad. Nên làm ở plan sau, khi frontend thực sự cần hiển thị ad.
- Backfill media cho các job cũ: link đã hết hạn, chỉ lấy lại được bằng cách crawl lại qua Apify.
- Dọn file trên đĩa / giới hạn dung lượng: ảnh 600px thường dưới 100KB, 10 nghìn ad khoảng 1GB.
