# Hướng dẫn tích hợp & vận hành Allegro Service (`Camoufox` + `YOLO26`)

Tài liệu này mô tả chi tiết kiến trúc, luồng dữ liệu, cơ chế vượt DataDome (Camoufox + YOLO26 ONNX), cấu trúc bảng database và cách tích hợp/triển khai **`allegro-service`** trên môi trường Local & VPS.

---

## 1. Kiến trúc tổng thể & Luồng dữ liệu

`allegro-service` được thiết kế đồng nhất 100% với các marketplace service khác trong hệ thống (`amazon-service`, `china1688-service`):

```
Client / Frontend
   │
   ├── 1. POST /jobs/allegro ────────► api-gateway (:8888)
   │                                       │
   │                                       ├─► INSERT jobs (type='allegro', status='pending')
   │                                       └─► Publish queue `allegro.jobs` (RabbitMQ)
   │                                                   │
   │                                                   ▼
   │                                           allegro-service (:8089)
   │                                                   │
   │                                                   ├─► Gọi subprocess `cmd/allegro-service/crawler/fetch.py`
   │                                                   │     ├── Khởi chạy Camoufox (os="linux", headless=True, humanize=True)
   │                                                   │     ├── Vượt DataDome Interstitial / Giải Slider Captcha bằng YOLO26
   │                                                   │     ├── Bóc tách HTML DOM (<article>, <dl><dt><dd>, aria-label)
   │                                                   │     └── (tuỳ chọn `--with-comments`) Mở từng product_url, bóc top-5
   │                                                   │         opinie theo lượt "pomocna" — best-effort, lỗi không fail job
   │                                                   │
   │                                                   ├─► INSERT từng sản phẩm vào bảng `allegro_raw` (PostgreSQL)
   │                                                   ├─► INSERT comment mỗi sản phẩm vào `allegro_comments`
   │                                                   ├─► UPDATE jobs SET status='crawled'
   │                                                   └─► Publish event `crawl.completed.allegro`
   │                                                               │
   │                                                               ▼
   │                                                           ai-service
   │                                                               │
   │                                                               ├─► Đọc `allegro_raw` -> Phân tích qua OpenRouter LLM
   │                                                               ├─► INSERT `ai_results`
   │                                                               └─► UPDATE jobs SET status='done'
   │
   └── 2. GET /jobs/allegro/:id ─────► api-gateway ──(gRPC)──► allegro-service (:8089)
                                       Trả về: { status, product_count, ai_output }
```

---

## 2. Cơ chế vượt DataDome & Giải Captcha (`Camoufox` + `YOLO26`)

Allegro.pl được bảo vệ bởi **DataDome**. Qua đo đạc thực tế trên hệ thống:

1. **Khớp OS/Kernel (`os="linux"`)**:
   - Nếu trình duyệt spoof `Windows NT 10.0` (`Win32`, Direct3D11) trong khi chạy trên nhân Linux (TCP/IP stack & X11/headless metrics của Linux), DataDome phát hiện mâu thuẫn OS/Kernel và lập tức trả về màn hình **"You have been blocked"** (`t=bv`).
   - Do đó, `fetch.py` và `run_allegro.py` tự động lấy đúng hệ điều hành thực của host (`host_os_name() -> "linux"`) để truyền vào `Camoufox(headless=True, os="linux", humanize=True)`.
2. **Phân biệt DataDome Interstitial (`rt: 'i'`) và Slider Captcha (`rt: 'c'`)**:
   - Khi mở trang lần đầu, DataDome trả về trang shell ~1.5KB chứa iframe `geo.captcha-delivery.com/interstitial/` (`rt: 'i'`).
   - Nếu giữ nguyên không can thiệp thô bạo vào DOM trong 4–6 giây đầu, script kiểm tra ngầm của DataDome tự xác thực thành công và tự redirect sang trang kết quả thật (~3.8MB HTML, ~60–77 sản phẩm/trang).
   - Chỉ khi DataDome trả về iframe `geo.captcha-delivery.com/captcha/` (`rt: 'c'`), bộ giải **`YOLO26CaptchaDetector` + `drag_slider` (`humanize=True`)** mới được kích hoạt để chụp vùng captcha, suy luận tọa độ `x_center` và kéo thanh trượt từ trái sang phải với quỹ đạo chuột tự nhiên.

---

## 3. Cấu trúc Database (`db/migrations/000012_allegro.up.sql`)

Bảng **`allegro_raw`** lưu trữ toàn bộ thông tin sản phẩm, nhà sản xuất, mẫu mã, giá và thông số kỹ thuật:

| Cột | Kiểu dữ liệu | Mô tả | Ví dụ thực tế |
|---|---|---|---|
| `id` | `UUID` | Khóa chính tự sinh | `gen_random_uuid()` |
| `job_id` | `UUID` | Tham chiếu `jobs(id)` | `...` |
| `offer_id` | `TEXT` | Mã offer trên Allegro | `"17118603300"` |
| `product_id` | `TEXT` | UUID sản phẩm trong catalog Allegro | `"880fe6be-3992-4532-97f2-8fd88c55889a"` |
| `title` | `TEXT` | Tên sản phẩm đầy đủ | `"Laptop Lenovo V15 G4 15,6 \" AMD Ryzen 5 16 GB / 512 GB czarny"` |
| `brand` | `TEXT` | **Nhà sản xuất / Thương hiệu** (`Marka` / `Producent`) | `"Lenovo"`, `"Dell"`, `"HP"`, `"Apple"` |
| `model` | `TEXT` | **Mẫu mã / Dòng máy** (`Model` / `Kod producenta`) | `"V15 G4"`, `"Inspiron i5"`, `"255R G10"` |
| `condition` | `TEXT` | Tình trạng (`Stan`) | `"Nowy"`, `"Używany"` |
| `seller` | `TEXT` | Tên nhà bán hàng | `"ogit_pl"` |
| `price` | `NUMERIC` | Giá hiện tại | `2499.00` |
| `price_with_delivery` | `NUMERIC` | Giá kèm phí vận chuyển (`z dostawą`) | `2509.49` |
| `currency` | `TEXT` | Đơn vị tiền tệ | `"PLN"` |
| `rating` | `NUMERIC` | Điểm đánh giá (trên thang 5) | `4.83` |
| `reviews_count` | `INTEGER` | Số lượng đánh giá (`ocen produktu`) | `113` |
| `bought_recently` | `INTEGER` | Số người mua gần đây (`osób kupiło ostatnio`) | `39` |
| `super_seller` | `BOOLEAN` | Huy hiệu `Super Sprzedawca` | `true` |
| `smart` | `BOOLEAN` | Hỗ trợ `Allegro Smart!` | `true` |
| `delivery` | `TEXT` | Thời gian giao hàng dự kiến | `"dostawa jutro – kup do 14:00"` |
| `sponsored` | `BOOLEAN` | Sản phẩm quảng cáo (`Promowane`) | `true` |
| `position` | `INTEGER` | Thứ tự xuất hiện trên trang | `1` |
| `page` | `INTEGER` | Số thứ tự trang kết quả | `1` |
| `image_url` | `TEXT` | Ảnh đại diện sản phẩm | `"https://a.allegroimg.com/s180/..."` |
| `product_url` | `TEXT` | Đường dẫn chi tiết sản phẩm | `"https://allegro.pl/produkt/..."` |
| `specs` | `JSONB` | **Thông số kỹ thuật bóc từ `<dl><dt><dd>`** | `{"Seria procesora": "AMD Ryzen 5", "Przekątna ekranu": "15.6\"", "Typ dysku twardego": "SSD"}` |
| `raw` | `JSONB` | Toàn bộ JSON gốc của card sản phẩm | `{...}` |

### Bảng `allegro_comments` (`db/migrations/000013_allegro_comments.up.sql`)

Mỗi sản phẩm lấy tối đa **5 comment** (opinie), ưu tiên comment nhiều lượt "pomocna" nhất. Quan hệ 1-nhiều với `allegro_raw` (`ON DELETE CASCADE`).

| Cột | Kiểu dữ liệu | Mô tả |
|---|---|---|
| `id` | `UUID` | Khóa chính |
| `allegro_raw_id` | `UUID` | FK → `allegro_raw(id)` (sản phẩm) |
| `job_id` | `UUID` | FK → `jobs(id)` |
| `offer_id` / `product_id` | `TEXT` | Tham chiếu offer/sản phẩm Allegro |
| `author` | `TEXT` | Tên người đánh giá (nếu có) |
| `rating` | `NUMERIC` | Số sao của review (thang 5) |
| `content` | `TEXT` | Nội dung review |
| `review_date` | `TEXT` | Ngày đăng (vd `"8 września 2025"`) |
| `confirmed_purchase` | `BOOLEAN` | Có huy hiệu "Opinia potwierdzona zakupem" |
| `helpful_count` | `INTEGER` | Số lượt "pomocna" (dùng để xếp hạng) |
| `pros` / `cons` | `TEXT` | Zalety / Wady (nếu có) |
| `position` | `INTEGER` | Thứ tự trong 5 comment lấy được |
| `raw` | `JSONB` | JSON gốc của comment |

---

## 4. API tích hợp qua `api-gateway` (`:8888`)

### 4.1. Tạo Job Crawl Allegro (`POST /jobs/allegro`)

Hỗ trợ tìm theo **từ khóa (`keyword`)**, **1 URL (`url`)**, hoặc **danh sách URL (`urls`)**:

```bash
# Cách 1: Tìm kiếm theo từ khóa
curl -X POST http://localhost:8888/jobs/allegro \
  -H "Content-Type: application/json" \
  -d '{
    "keyword": "laptop",
    "pages": 1,
    "max_items": 30,
    "with_comments": true,
    "max_comments": 5
  }'

# Cách 2: Crawl trực tiếp URL listing Allegro (có sẵn bộ lọc/sort)
curl -X POST http://localhost:8888/jobs/allegro \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://allegro.pl/listing?string=fotel%20gamingowy&order=qd",
    "pages": 2,
    "max_items": 60
  }'
```

**Response:**
```json
{
  "job_id": "c4f1d2a8-7b12-4e9a-8d3e-1a2b3c4d5e6f"
}
```

> `with_comments` mặc định **bật** khi không truyền (job sẽ lấy thêm comment).
> Gửi `"with_comments": false` để chỉ crawl listing như trước. `max_comments`
> mặc định **5**.

### 4.2. Kiểm tra trạng thái & Kết quả Job (`GET /jobs/allegro/:id`)

```bash
curl http://localhost:8888/jobs/allegro/c4f1d2a8-7b12-4e9a-8d3e-1a2b3c4d5e6f
```

**Response:**
```json
{
  "status": "done",
  "product_count": 30,
  "ai_output": "The Allegro laptop market for this query ranges from 949 PLN for entry-level Intel Celeron models to 5,576 PLN for Apple MacBook Air M5 configurations, with mainstream 15.6-inch AMD Ryzen 5 and Intel Core i5 laptops from Lenovo, HP, Dell, and ASUS clustering around 2,200–2,900 PLN..."
}
```

---

## 5. Cài đặt & Triển khai trên VPS (Gọn nhẹ, không tốn ổ cứng)

> **Lưu ý quan trọng trên VPS:** Không cài gói `ultralytics` mặc định từ PyPI vì nó sẽ tự kéo bộ thư viện card đồ họa NVIDIA CUDA (`nvidia-cublas`, `nvidia-cudnn`...) nặng ~3GB làm đầy ổ cứng VPS. Chỉ cần cài bộ thư viện chạy CPU gọn nhẹ (~60MB) dưới đây.

### Bước 1: Cài đặt môi trường Python & Camoufox (Chạy từng dòng ngắn)

```bash
cd /var/www/crawl_ecomerce_golang/scripts/camoufox-yolo-captcha
python3 -m venv .venv
./.venv/bin/pip install --no-cache-dir opencv-python-headless
./.venv/bin/pip install --no-cache-dir camoufox
./.venv/bin/pip install --no-cache-dir onnxruntime python-dotenv
./.venv/bin/python -m camoufox fetch
```

### Bước 2: Chạy Migration PostgreSQL

```bash
cd /var/www/crawl_ecomerce_golang
psql "postgres://crawl:crawl@localhost:5433/crawl?sslmode=disable" -f db/migrations/000012_allegro.up.sql
```

### Bước 3: Cấu hình `cmd/allegro-service/etc/allegro.yaml`

Đảm bảo đường dẫn `Python` trỏ tới đúng `.venv` trên máy/VPS:

```yaml
Name: allegro-service
ListenOn: 0.0.0.0:8089
Postgres:
  DSN: postgres://crawl:crawl@localhost:5433/crawl?sslmode=disable
RabbitMQ:
  URL: amqp://guest:guest@localhost:5673/
  Exchange: crawl
  Concurrency: 1
Allegro:
  Python: /var/www/crawl_ecomerce_golang/scripts/camoufox-yolo-captcha/.venv/bin/python
  Script: cmd/allegro-service/crawler/fetch.py
  Proxy: ${ALLEGRO_PROXY}          # 1 proxy tĩnh (tuỳ chọn)
  ProxyFile: ${ALLEGRO_PROXY_FILE} # file proxy Webshare để xoay vòng (tuỳ chọn)
  Xvfb: false
  TimeoutSeconds: 300
```

#### Vượt DataDome bằng proxy (Webshare)

Allegro chặn IP datacenter/VPS bằng DataDome (trang interstitial ~2KB `allegrocaptcha.com`). Dùng proxy residential để vượt:

- **`WEBSHARE_API_KEY`** (env): service gọi thẳng **Webshare API** lấy pool proxy mới nhất rồi xoay vòng + retry. **Ưu tiên cao nhất**, không cần mount file. Khuyến nghị cho VPS/Docker.
- **`ProxyFile`** trỏ tới file Webshare (mỗi dòng `host:port:user:pass`). Service chọn **ngẫu nhiên 1 proxy**, nếu bị chặn thì **tự thử proxy kế tiếp** (tối đa `--proxy-retries`, mặc định 5).
- **`Proxy`** dùng 1 proxy tĩnh duy nhất (không xoay vòng).
- Không set gì → crawl trực tiếp (thường bị DataDome chặn trên VPS).

Thứ tự ưu tiên nguồn proxy: **API key → ProxyFile → Proxy → direct** (nguồn sau chỉ dùng khi nguồn trước rỗng). API key truyền qua **env** (không lộ trong `ps`); CLI độc lập dùng `--proxy-api-key` hoặc env `WEBSHARE_API_KEY`:

```bash
WEBSHARE_API_KEY=xxxxxxxx ./scripts/camoufox-yolo-captcha/.venv/bin/python \
  cmd/allegro-service/crawler/fetch.py --keyword "laptop" --pages 1 --max-items 2 --with-comments
```

CLI độc lập có proxy rotation:
```bash
./scripts/camoufox-yolo-captcha/.venv/bin/python cmd/allegro-service/crawler/fetch.py \
  --keyword "laptop" --pages 1 --max-items 2 --with-comments \
  --proxy-file "scripts/proxy-test/Webshare 100 proxies.txt" --proxy-retries 5
```

> Docker: set `ALLEGRO_PROXY_FILE` là path **trong container** và mount file proxy host vào đó (xem block `volumes` đã comment sẵn trong `docker-compose.yml`).

### Bước 4: Khởi chạy Service hoặc Chạy CLI độc lập

- **Chạy Microservice (`allegro-service`):**
  ```bash
  go run ./cmd/allegro-service -f cmd/allegro-service/etc/allegro.yaml
  ```
- **Chạy CLI độc lập (`fetch.py` trả JSON ra stdout):**
  ```bash
  ./scripts/camoufox-yolo-captcha/.venv/bin/python cmd/allegro-service/crawler/fetch.py --keyword "laptop" --pages 1 --max-items 10
  ```
- **Chạy Script xuất file JSON (`run_allegro.py`):**
  ```bash
  ./scripts/camoufox-yolo-captcha/.venv/bin/python scripts/camoufox-yolo-captcha/run_allegro.py "laptop" -p 1
  ```
