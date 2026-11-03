# Tích hợp YOLO26 + Camoufox giải Slider Captcha

Bộ công cụ triển khai đầy đủ 4 giai đoạn tích hợp **YOLO26 (ONNX)** + **Camoufox (`humanize=True`)** để phát hiện khe hở (gap) của Slider Captcha và thực hiện thao tác kéo thả với chuyển động chuột tự nhiên.

## Cấu trúc thư mục

- [`train_and_export.py`](./train_and_export.py) — **Giai đoạn 1**: Tự động sinh bộ dataset Slider Captcha mẫu (hoặc nhận dataset thực tế qua `--data`), huấn luyện mô hình `yolo26n.pt` và xuất sang định dạng ONNX (`models/yolo26n_captcha.onnx`).
- [`captcha_solver.py`](./captcha_solver.py) — **Giai đoạn 2 & 3**:
  - `YOLO26CaptchaDetector`: Nạp model `yolo26n_captcha.onnx` (kèm fallback OpenCV Canny/Contour) để suy luận tọa độ `x_center` của khe hở từ ảnh chụp phần tử `#captcha-bg`.
  - `drag_slider()`: Thực hiện chuỗi hành động `move -> down -> move -> up` kết hợp `humanize=True` của Camoufox.
  - `solve_slider_captcha()`: Hàm điều phối trọn luồng từ chụp ảnh phần tử DOM, quy đổi tỷ lệ hiển thị CSS, tính `distance_x` đến kéo thanh trượt.
- [`benchmark_runner.py`](./benchmark_runner.py) — **Giai đoạn 4**: Chạy vòng lặp kiểm thử tự động 30–50 trials độc lập trên Local Slider Captcha Testbed (hoặc URL thực tế qua `--url`), kiểm tra cả sai số vị trí pixel lẫn hành vi quỹ đạo chuột chống bot, và xuất báo cáo `benchmark_results.json`.

## Hướng dẫn cài đặt & sử dụng

### 1. Chuẩn bị môi trường (Giai đoạn 1.2)

```bash
python3 -m venv .venv
./.venv/bin/pip install -r requirements.txt
./.venv/bin/camoufox fetch
```

### 2. Huấn luyện & Xuất mô hình YOLO26 sang ONNX (Giai đoạn 1.1)

```bash
# Tự sinh dataset Slider Captcha mẫu, huấn luyện yolo26n.pt và xuất sang ONNX:
./.venv/bin/python train_and_export.py --epochs 15 --samples 160

# Hoặc huấn luyện trên dataset gán nhãn của bạn:
./.venv/bin/python train_and_export.py --data /path/to/your_captcha_data.yaml --epochs 100 --imgsz 320
```

### 3. Chạy kiểm thử & Đo lường 30–50 lần (Giai đoạn 4)

```bash
# Chạy 30 trials ở chế độ headless với Camoufox humanize=True:
./.venv/bin/python benchmark_runner.py --trials 30

# Mở trình duyệt Camoufox hiển thị trực quan (headed):
./.venv/bin/python benchmark_runner.py --trials 10 --headed

# So sánh khi tắt humanize (sẽ bị bộ lọc hành vi chuột phát hiện):
./.venv/bin/python benchmark_runner.py --trials 10 --no-humanize
```

Kết quả đo đạc chi tiết được lưu tại `benchmark_results.json` và ảnh chụp minh họa lưu tại `debug/trial_01_result.png`.
