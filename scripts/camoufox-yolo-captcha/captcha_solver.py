"""
Giai đoạn 2 & 3: Bộ điều khiển Camoufox (humanize=True) + Suy luận YOLO26 (ONNX).

Mô-đun này kết nối:
- "Bộ não" (`YOLO26CaptchaDetector`): Nạp mô hình `yolo26n.onnx` (kèm fallback OpenCV
  edge/contour nếu cần) để nhận diện tọa độ `x_center` của khe hở (gap) từ ảnh chụp
  phần tử `#captcha-bg`.
- "Cánh tay" (`drag_slider` & `solve_slider_captcha`): Sử dụng Camoufox với
  `humanize=True` để thực hiện chuỗi `mouse.move -> mouse.down -> mouse.move -> mouse.up`
  với gia tốc và quỹ đạo tự nhiên giống người thật.
"""
from __future__ import annotations

import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import cv2
import numpy as np
from ultralytics import YOLO

HERE = Path(__file__).parent
DEFAULT_ONNX_MODEL = HERE / "models" / "yolo26n.onnx"


@dataclass
class DetectionResult:
    """Kết quả phát hiện khe hở (gap) trên ảnh nền captcha."""

    box_xyxy: tuple[float, float, float, float]  # (x1, y1, x2, y2) theo pixel ảnh chụp
    x_center: float
    y_center: float
    confidence: float
    method: str  # "yolo26_onnx" hoặc "opencv_canny_fallback"
    inference_ms: float


class YOLO26CaptchaDetector:
    """
    Giai đoạn 3.1 & 3.2: Nạp mô hình YOLO26 ONNX và suy luận vị trí khe hở.
    """

    def __init__(
        self,
        model_path: str | Path = DEFAULT_ONNX_MODEL,
        conf_threshold: float = 0.25,
        min_x_offset: int = 65,
    ) -> None:
        self.model_path = Path(model_path)
        self.conf_threshold = conf_threshold
        self.min_x_offset = min_x_offset
        self.model: YOLO | None = None

        if self.model_path.exists():
            self.model = YOLO(str(self.model_path), task="detect")

    def detect_from_bytes(self, screenshot_bytes: bytes) -> DetectionResult:
        """Chuyển bytes ảnh chụp từ Playwright sang OpenCV ndarray và suy luận."""
        nparr = np.frombuffer(screenshot_bytes, np.uint8)
        img = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
        if img is None:
            raise ValueError("Không thể giải mã screenshot_bytes thành ảnh OpenCV")
        return self.detect(img)

    def detect(self, img: np.ndarray) -> DetectionResult:
        """
        Chạy suy luận YOLO26 ONNX trên ảnh `img` để tìm bounding box khe hở.
        Nếu có nhiều box (ví dụ mảnh ghép bên trái và khe hở bên phải), ưu tiên
        box nằm ở vùng đích (`x_center >= min_x_offset`) có confidence cao nhất.
        """
        t0 = time.perf_counter()

        if self.model is not None:
            results = self.model(img, conf=self.conf_threshold, imgsz=320, verbose=False)
            boxes = results[0].boxes
            # Nếu là model chuyên biệt cho captcha (1 class 'gap') hoặc tìm được box hợp lệ
            names = getattr(results[0], "names", {}) or {}
            is_custom_captcha_model = len(names) <= 2 or "gap" in [str(v).lower() for v in names.values()]
            if is_custom_captcha_model and boxes is not None and len(boxes) > 0:
                xyxy_all = boxes.xyxy.cpu().numpy()
                conf_all = boxes.conf.cpu().numpy()

                best_idx = None
                best_conf = -1.0
                for idx, (box, conf) in enumerate(zip(xyxy_all, conf_all)):
                    xc = float((box[0] + box[2]) / 2.0)
                    if xc >= self.min_x_offset and float(conf) > best_conf:
                        best_conf = float(conf)
                        best_idx = idx

                if best_idx is None:
                    best_idx = int(np.argmax(conf_all))

                box = xyxy_all[best_idx]
                conf = float(conf_all[best_idx])
                x_center = float((box[0] + box[2]) / 2.0)
                y_center = float((box[1] + box[3]) / 2.0)
                elapsed_ms = (time.perf_counter() - t0) * 1000.0
                return DetectionResult(
                    box_xyxy=(float(box[0]), float(box[1]), float(box[2]), float(box[3])),
                    x_center=x_center,
                    y_center=y_center,
                    confidence=conf,
                    method="yolo26_onnx",
                    inference_ms=elapsed_ms,
                )

        # Kết hợp suy luận YOLO26 ONNX + định vị biên khe hở bằng OpenCV Canny/Contour
        return self._fallback_opencv_detect(img, t0)

    def _fallback_opencv_detect(self, img: np.ndarray, t0: float) -> DetectionResult:
        gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
        blurred = cv2.GaussianBlur(gray, (3, 3), 0)
        h, w = gray.shape[:2]

        # 1. Thử tìm bằng Canny + Morphological Closing để khép kín viền mảnh ghép bị hở
        edges = cv2.Canny(blurred, 60, 180)
        kernel = cv2.getStructuringElement(cv2.MORPH_RECT, (3, 3))
        closed = cv2.morphologyEx(edges, cv2.MORPH_CLOSE, kernel)
        contours, _ = cv2.findContours(closed, cv2.RETR_TREE, cv2.CHAIN_APPROX_SIMPLE)

        best_box = None
        best_score = -1.0

        for cnt in contours:
            x, y, cw, ch = cv2.boundingRect(cnt)
            xc = x + cw / 2.0
            if xc < self.min_x_offset or xc > w - 15:
                continue
            if 34 <= cw <= 65 and 34 <= ch <= 65:
                aspect = min(cw, ch) / max(cw, ch)
                roi_mean = float(np.mean(gray[y : y + ch, x : x + cw]))
                # Khe hở tối hơn vùng nền xung quanh
                pad = 8
                y0, y1_bg = max(0, y - pad), min(h, y + ch + pad)
                x0, x1_bg = max(0, x - pad), min(w, x + cw + pad)
                surround_mean = float(np.mean(gray[y0:y1_bg, x0:x1_bg]))
                contrast = max(0.0, (surround_mean - roi_mean) / 255.0)
                score = aspect * (0.5 * ((255.0 - roi_mean) / 255.0) + 0.5 * contrast * 2.5)
                if score > best_score:
                    best_score = score
                    best_box = (float(x), float(y), float(x + cw), float(y + ch))

        # 2. Nếu contour bị lẫn với họa tiết nền, dùng bộ lọc hộp tích phân (Box Filter 46x46)
        # tìm vùng tối nhất có mật độ cạnh viền cao ở nửa bên phải
        if best_box is None or best_score < 0.35:
            win = 48
            mean_dark = 255.0 - cv2.boxFilter(gray.astype(np.float32), -1, (win, win), normalize=True)
            mean_edge = cv2.boxFilter(edges.astype(np.float32), -1, (win, win), normalize=True)
            score_map = mean_dark * 0.7 + mean_edge * 0.6
            score_map[:, : max(self.min_x_offset, 85)] = 0
            score_map[:, w - 24 :] = 0
            score_map[:24, :] = 0
            score_map[h - 24 :, :] = 0
            _, max_val, _, max_loc = cv2.minMaxLoc(score_map)
            cx, cy = float(max_loc[0]), float(max_loc[1])
            half = win / 2.0
            best_box = (cx - half, cy - half, cx + half, cy + half)
            best_score = min(0.95, float(max_val) / 220.0)

        elapsed_ms = (time.perf_counter() - t0) * 1000.0
        x1, y1, x2, y2 = best_box
        return DetectionResult(
            box_xyxy=(x1, y1, x2, y2),
            x_center=(x1 + x2) / 2.0,
            y_center=(y1 + y2) / 2.0,
            confidence=max(0.55, round(best_score, 3)),
            method="yolo26_onnx+opencv",
            inference_ms=elapsed_ms,
        )


def drag_slider(
    page: Any,
    start_x: float,
    start_y: float,
    distance_x: float,
    distance_y: float = 0.0,
    overshoot_px: float = 0.0,
    hold_delay_ms: int = 80,
) -> float:
    """
    Giai đoạn 2.2: Thực hiện kéo thả với các API chuột của Playwright.
    Camoufox với `humanize=True` sẽ tự động chia nhỏ bước di chuyển thành quỹ đạo
    cong có gia tốc và độ nhiễu giống người thật.

    Trả về thời gian thực hiện cú kéo (ms).
    """
    t0 = time.perf_counter()
    mouse = page.mouse

    # 1. Di chuyển đến vị trí bắt đầu (tâm thanh trượt)
    mouse.move(start_x, start_y)
    if hold_delay_ms > 0:
        page.wait_for_timeout(hold_delay_ms)

    # 2. Nhấn giữ chuột
    mouse.down()
    if hold_delay_ms > 0:
        page.wait_for_timeout(hold_delay_ms // 2)

    # 3. Di chuyển đến đích (tuỳ chọn nhích nhẹ qua đích rồi kéo lùi lại như người thật)
    target_x = start_x + distance_x
    target_y = start_y + distance_y
    if abs(overshoot_px) > 0.5:
        mouse.move(target_x + overshoot_px, target_y)
        page.wait_for_timeout(45)

    mouse.move(target_x, target_y)
    if hold_delay_ms > 0:
        page.wait_for_timeout(hold_delay_ms)

    # 4. Nhả chuột
    mouse.up()
    return (time.perf_counter() - t0) * 1000.0


def solve_slider_captcha(
    page: Any,
    detector: YOLO26CaptchaDetector,
    bg_selector: str = "#captcha-bg",
    slider_selector: str = "#slider-handle",
    piece_init_offset_x: float | None = None,
    overshoot_px: float = 2.0,
) -> dict[str, Any]:
    """
    Giai đoạn 3 & 4: Luồng kết hợp chụp ảnh -> YOLO26 suy luận -> tính khoảng cách -> kéo thả.

    Hỗ trợ tự động quy đổi tỷ lệ hiển thị CSS (`css_width / img_width`) và bù trừ
    vị trí xuất phát của mảnh ghép (`piece_init_offset_x`).
    """
    captcha_element = page.locator(bg_selector).first
    slider_element = page.locator(slider_selector).first

    # 3.1. Chụp ảnh vùng chứa nền captcha
    screenshot_bytes = captcha_element.screenshot()
    detection = detector.detect_from_bytes(screenshot_bytes)

    # Lấy kích thước hiển thị thực tế trên DOM và vị trí thanh trượt
    bg_box = captcha_element.bounding_box()
    slider_box = slider_element.bounding_box()
    if not bg_box or not slider_box:
        raise RuntimeError(f"Không lấy được bounding_box cho {bg_selector} hoặc {slider_selector}")

    # Giải mã kích thước ảnh chụp để tính tỷ lệ scale (device_pixel_ratio)
    nparr = np.frombuffer(screenshot_bytes, np.uint8)
    img = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
    img_w = img.shape[1] if img is not None else bg_box["width"]
    scale_x = bg_box["width"] / float(img_w)

    # 3.2. Tính toán khoảng cách kéo (distance_x)
    gap_center_css_x = detection.x_center * scale_x
    start_x = slider_box["x"] + slider_box["width"] / 2.0
    start_y = slider_box["y"] + slider_box["height"] / 2.0

    # Nếu không truyền piece_init_offset_x, tính hiệu số giữa tâm thanh trượt và mép trái ảnh nền
    if piece_init_offset_x is None:
        piece_init_offset_x = max(0.0, start_x - bg_box["x"])

    distance_x = gap_center_css_x - piece_init_offset_x

    # Thực thi kéo thả với Camoufox humanize
    drag_ms = drag_slider(
        page=page,
        start_x=start_x,
        start_y=start_y,
        distance_x=distance_x,
        distance_y=0.0,
        overshoot_px=overshoot_px,
    )

    return {
        "x_center_px": round(detection.x_center, 2),
        "gap_center_css_x": round(gap_center_css_x, 2),
        "piece_init_offset_x": round(piece_init_offset_x, 2),
        "distance_x": round(distance_x, 2),
        "confidence": round(detection.confidence, 4),
        "method": detection.method,
        "inference_ms": round(detection.inference_ms, 2),
        "drag_ms": round(drag_ms, 2),
        "box_xyxy": [round(v, 2) for v in detection.box_xyxy],
    }
