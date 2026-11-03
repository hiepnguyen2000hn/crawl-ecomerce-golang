# Deploy: lưu ảnh FB ads về VPS

Link ảnh fbcdn mà Apify trả về có chữ ký và hết hạn sau vài ngày (tham số
`oe=`; quá hạn fbcdn trả 403). Sau mỗi job crawl, `fb-ads-service` tải ảnh
SD của từng ad (`resized_image_url`) và ảnh bìa của ad video
(`video_preview_image_url`) về thư mục `MEDIA_HOST_DIR` trên VPS. nginx
phục vụ thư mục này tại `/media/`. Không tải file video.

- Trạng thái từng ảnh nằm ở bảng `fbads_media`: `pending` / `done` / `failed` / `expired`.
- Đường dẫn file: `<MEDIA_HOST_DIR>/<storage_key>`, ví dụ
  `/srv/crawl/media/fbads/2179652559429754/snapshot_images_0_resized_image_url.jpg`.
- URL công khai: `https://<domain>/media/<storage_key>`.
- `fbads_raw.raw` giữ nguyên, vẫn chứa link fbcdn gốc.

## Các bước deploy

1. Lấy code mới:
   ```bash
   cd <repo> && git pull
   ```
2. Tạo thư mục lưu ảnh (nginx cần quyền đọc):
   ```bash
   sudo mkdir -p /srv/crawl/media
   sudo chmod 755 /srv/crawl /srv/crawl/media
   ```
3. Thêm vào `.env`:
   ```
   MEDIA_HOST_DIR=/srv/crawl/media
   ```
4. Build lại service. Service `migrate` tự chạy migration `000011_fbads_media` trước:
   ```bash
   docker compose up -d --build fb-ads-service
   docker compose logs migrate | tail -3        # thấy "11/u fbads_media"
   docker compose logs fb-ads-service | tail    # không có panic về media dir
   ```
5. Cấu hình nginx:
   ```bash
   sudo cp deploy/nginx/fbads-media.conf /etc/nginx/snippets/
   # Thêm dòng sau vào server { } của domain:
   #     include snippets/fbads-media.conf;
   sudo nginx -t && sudo systemctl reload nginx
   ```
   Nếu `MEDIA_HOST_DIR` khác `/srv/crawl/media` thì sửa `alias` trong snippet
   cho khớp (giữ dấu `/` ở cuối).
6. Kiểm tra sau khi chạy một job fbads:
   ```bash
   docker compose exec postgres psql -U crawl -c \
     "select status, count(*) from fbads_media group by 1"
   docker compose exec postgres psql -U crawl -Atc \
     "select storage_key from fbads_media where status='done' limit 1"
   curl -sI https://<domain>/media/<storage_key>   # 200, content-type: image/jpeg
   curl -sI https://<domain>/media/fbads/          # 404, không liệt kê thư mục
   ```

## Backup

```bash
rsync -a /srv/crawl/media/ <backup-host>:/backup/crawl-media/
```
Mỗi file chỉ được ghi một lần, không bị sửa sau đó, nên chạy rsync hằng ngày
bằng cron là đủ. Ảnh 600px thường dưới 100KB, 10 nghìn ad tốn khoảng 1GB.

## Theo dõi

```sql
-- Ảnh lỗi gần đây (failed = lỗi mạng sau 3 lần thử; expired = fbcdn trả 403/404/410)
SELECT status, error, count(*) FROM fbads_media
WHERE updated_at > now() - interval '1 day' AND status IN ('failed', 'expired')
GROUP BY 1, 2 ORDER BY 3 DESC;
```
Cứ 15 phút service tự quét lại một lần: ảnh `pending` mà quá 15 phút không ai
xử lý (ví dụ worker restart giữa chừng), và ảnh `failed` có `attempts < 5`,
đều được tải lại nếu link còn hạn. Một job quá lớn chưa tải xong trong 10 phút
sẽ được RabbitMQ giao lại để tải nốt phần còn lại.

Nếu `expired` nhiều ngay sau khi crawl, có thể fbcdn đang chặn IP của VPS. Thử
`curl -I <source_url>` từ VPS. Một ad bị crawl lại ở job sau sẽ có link mới:
dòng `failed`/`expired` tự quay về `pending` và được tải lại, dòng `done` giữ
nguyên.

## Rollback

```bash
git checkout <commit trước> && docker compose up -d --build fb-ads-service
```
Không cần migrate down: code cũ không dùng tới bảng `fbads_media`. Thư mục ảnh
giữ nguyên.
