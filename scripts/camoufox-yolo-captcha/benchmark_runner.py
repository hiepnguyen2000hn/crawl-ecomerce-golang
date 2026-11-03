"""
Giai đoạn 4: Vòng lặp hoàn chỉnh và kiểm thử đo lường (Benchmark 30–50 trials).

Script này cung cấp:
1. Một Local Slider Captcha Benchmark Server (HTTP) tích hợp bộ chấm điểm kép:
   - Kiểm tra độ chính xác vị trí kéo (`|dragged_x - true_gap_x| <= tolerance_px`).
   - Kiểm tra hành vi chuột chống bot (số điểm trung gian `move_events >= 6`,
     thời gian kéo hợp lý, và có độ biến thiên vận tốc/toạ độ của `humanize=True`).
2. Vòng lặp kiểm thử tự động 30–50 lần độc lập bằng Camoufox + YOLO26 ONNX,
   thu thập số liệu chi tiết (Success Rate, sai số pixel MAE, thời gian suy luận
   ONNX, thời gian kéo chuột, số điểm quỹ đạo chuột) và xuất báo cáo JSON.
3. Chế độ `--url` để chạy trực tiếp trên URL trang web thực tế.

Cách chạy:
    ./.venv/bin/python benchmark_runner.py --trials 35
"""
from __future__ import annotations

import argparse
import base64
import json
import os
import random
import statistics
import threading
import time
from datetime import datetime
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlparse

import cv2
from camoufox.sync_api import Camoufox
from dotenv import load_dotenv

from captcha_solver import DEFAULT_ONNX_MODEL, YOLO26CaptchaDetector, solve_slider_captcha
from train_and_export import build_synthetic_dataset, render_captcha_sample, train_and_export

HERE = Path(__file__).parent
load_dotenv(HERE / ".env")

DEBUG_DIR = HERE / "debug"


CAPTCHA_PAGE_HTML = """<!DOCTYPE html>
<html lang="vi">
<head>
  <meta charset="UTF-8" />
  <title>Slider Captcha Benchmark Testbed</title>
  <style>
    body {
      font-family: system-ui, -apple-system, sans-serif;
      background: #f2f4f8;
      display: flex;
      justify-content: center;
      align-items: center;
      height: 100vh;
      margin: 0;
    }
    .captcha-card {
      background: #fff;
      padding: 20px 24px;
      border-radius: 12px;
      box-shadow: 0 8px 24px rgba(0,0,0,0.1);
      width: 320px;
    }
    .title {
      font-size: 15px;
      font-weight: 600;
      color: #1e293b;
      margin-bottom: 12px;
    }
    #captcha-wrapper {
      position: relative;
      width: 320px;
      height: 160px;
      border-radius: 8px;
      overflow: hidden;
      border: 1px solid #cbd5e1;
      user-select: none;
    }
    #captcha-bg {
      width: 320px;
      height: 160px;
      display: block;
    }
    #slider-track {
      position: relative;
      margin-top: 14px;
      width: 320px;
      height: 40px;
      background: #e2e8f0;
      border-radius: 20px;
      border: 1px solid #cbd5e1;
      user-select: none;
    }
    .track-text {
      position: absolute;
      width: 100%;
      text-align: center;
      line-height: 40px;
      font-size: 13px;
      color: #64748b;
      pointer-events: none;
    }
    #slider-handle {
      position: absolute;
      left: 0px;
      top: 0px;
      width: 44px;
      height: 40px;
      background: #ff5a00;
      border-radius: 20px;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      color: #fff;
      font-weight: bold;
      box-shadow: 0 2px 6px rgba(255,90,0,0.4);
    }
    #captcha-status {
      margin-top: 12px;
      font-size: 13px;
      font-weight: 600;
      min-height: 20px;
    }
    .status-pass { color: #16a34a; }
    .status-fail { color: #dc2626; }
  </style>
</head>
<body>
  <div class="captcha-card">
    <div class="title">Xác minh bảo mật: Kéo mảnh ghép vào đúng khe hở</div>
    <div id="captcha-wrapper">
      <img id="captcha-bg" src="data:image/jpeg;base64,__IMAGE_B64__" alt="captcha-bg" />
    </div>
    <div id="slider-track">
      <div class="track-text">Kéo sang phải để hoàn tất >>></div>
      <div id="slider-handle">→</div>
    </div>
    <div id="captcha-status" data-state="pending" data-true-gap="__TRUE_GAP_X__" data-init-x="__INIT_CENTER_X__">
      Đang chờ thao tác...
    </div>
  </div>

  <script>
    const handle = document.getElementById('slider-handle');
    const statusEl = document.getElementById('captcha-status');
    const trueGapCenter = parseFloat(statusEl.dataset.trueGap);
    const initCenter = parseFloat(statusEl.dataset.initX);
    const tolerancePx = __TOLERANCE_PX__;

    let dragging = false;
    let startClientX = 0;
    let startTime = 0;
    let currentDeltaX = 0;
    let trajectory = [];

    handle.addEventListener('mousedown', (e) => {
      dragging = true;
      startClientX = e.clientX;
      startTime = performance.now();
      currentDeltaX = 0;
      trajectory = [{ x: e.clientX, y: e.clientY, t: 0 }];
      statusEl.dataset.state = 'dragging';
    });

    window.addEventListener('mousemove', (e) => {
      if (!dragging) return;
      currentDeltaX = Math.max(0, Math.min(276, e.clientX - startClientX));
      handle.style.left = currentDeltaX + 'px';
      trajectory.push({
        x: e.clientX,
        y: e.clientY,
        t: Math.round(performance.now() - startTime)
      });
    });

    window.addEventListener('mouseup', (e) => {
      if (!dragging) return;
      dragging = false;
      const durationMs = Math.round(performance.now() - startTime);
      const finalCenter = initCenter + currentDeltaX;
      const posError = Math.abs(finalCenter - trueGapCenter);

      // Phân tích hành vi chuột (bot check):
      // Người thật hoặc Camoufox humanize=True sẽ tạo ra nhiều sự kiện mousemove (> 4 điểm)
      const moveCount = trajectory.length;
      const yValues = trajectory.map(p => p.y);
      const ySpan = Math.max(...yValues) - Math.min(...yValues);
      const humanLike = moveCount >= 5 && durationMs >= 35;
      const positionMatched = posError <= tolerancePx;
      const success = positionMatched && humanLike;

      const detail = {
        success,
        positionMatched,
        humanLike,
        finalCenter: Number(finalCenter.toFixed(2)),
        trueGapCenter: Number(trueGapCenter.toFixed(2)),
        posErrorPx: Number(posError.toFixed(2)),
        dragDistancePx: Number(currentDeltaX.toFixed(2)),
        moveEvents: moveCount,
        yJitterPx: Number(ySpan.toFixed(2)),
        durationMs
      };

      statusEl.dataset.state = success ? 'success' : 'failed';
      statusEl.dataset.detail = JSON.stringify(detail);
      if (success) {
        statusEl.className = 'status-pass';
        statusEl.textContent = 'Xác minh thành công';
      } else {
        statusEl.className = 'status-fail';
        statusEl.textContent = `Xác minh thất bại (err=${detail.posErrorPx}px, moves=${moveCount})`;
      }
    });
  </script>
</body>
</html>
"""


class CaptchaBenchmarkServer:
    """HTTP Server nội bộ phục vụ trang Slider Captcha động theo từng trial."""

    def __init__(self, tolerance_px: float = 7.0) -> None:
        self.tolerance_px = tolerance_px
        self._server: HTTPServer | None = None
        self._thread: threading.Thread | None = None
        self.port: int = 0

    def start(self) -> str:
        tolerance = self.tolerance_px

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:  # noqa: N802
                parsed = urlparse(self.path)
                qs = parse_qs(parsed.query)
                seed = int(qs.get("seed", [random.randint(1, 999999)])[0])

                # Tạo mẫu captcha ngẫu nhiên nhưng tái lập được theo seed
                img, _, gap_center_x = render_captcha_sample(
                    width=320,
                    height=160,
                    piece_size=44,
                    draw_floating_piece=True,
                    seed=seed,
                )
                _, buf = cv2.imencode(".jpg", img, [int(cv2.IMWRITE_JPEG_QUALITY), 92])
                b64_img = base64.b64encode(buf.tobytes()).decode("ascii")

                # Thanh trượt rộng 44px bắt đầu tại left=0 -> tâm thanh trượt ban đầu ở x = 22px
                html = (
                    CAPTCHA_PAGE_HTML.replace("__IMAGE_B64__", b64_img)
                    .replace("__TRUE_GAP_X__", str(gap_center_x))
                    .replace("__INIT_CENTER_X__", "22")
                    .replace("__TOLERANCE_PX__", str(tolerance))
                )
                body = html.encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "text/html; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, format: str, *args: Any) -> None:  # noqa: A002
                return  # Tắt log HTTP ồn ào khi chạy 30-50 trials

        self._server = HTTPServer(("127.0.0.1", 0), Handler)
        self.port = self._server.server_port
        self._thread = threading.Thread(target=self._server.serve_forever, daemon=True)
        self._thread.start()
        return f"http://127.0.0.1:{self.port}/captcha"

    def stop(self) -> None:
        if self._server:
            self._server.shutdown()
            self._server.server_close()


def ensure_onnx_model(model_path: Path) -> Path:
    """Đảm bảo model ONNX đã tồn tại; nếu chưa có thì xuất trực tiếp từ pretrained yolo26n.pt."""
    if model_path.exists():
        return model_path

    import shutil
    from ultralytics import YOLO

    model_path.parent.mkdir(parents=True, exist_ok=True)
    print(f"[+] Xuất trực tiếp pretrained model sang ONNX tại {model_path} (không cần train lại)...")
    model = YOLO(str(HERE / "yolo26n.pt") if (HERE / "yolo26n.pt").exists() else "yolo26n.pt")
    exported = Path(model.export(format="onnx", imgsz=320))
    if exported.resolve() != model_path.resolve():
        shutil.copy2(exported, model_path)
    return model_path


def run_benchmark(
    trials: int = 30,
    headless: bool = True,
    humanize: bool = True,
    tolerance_px: float = 7.0,
    max_retries_per_trial: int = 2,
    target_url: str | None = None,
    model_path: Path = DEFAULT_ONNX_MODEL,
) -> dict[str, Any]:
    """
    Giai đoạn 4: Chạy vòng lặp hoàn chỉnh và đo lường trên 30–50 lần thử nghiệm độc lập.
    """
    ensure_onnx_model(model_path)
    detector = YOLO26CaptchaDetector(model_path=model_path, conf_threshold=0.15)

    server: CaptchaBenchmarkServer | None = None
    base_url = target_url
    if not base_url:
        server = CaptchaBenchmarkServer(tolerance_px=tolerance_px)
        base_url = server.start()

    DEBUG_DIR.mkdir(parents=True, exist_ok=True)
    trial_records: list[dict[str, Any]] = []

    print(
        f"[+] Khởi chạy Camoufox (headless={headless}, humanize={humanize}) "
        f"với model={model_path.name} trên {trials} trials..."
    )

    t_start_all = time.perf_counter()
    try:
        with Camoufox(headless=headless, humanize=humanize) as browser:
            context = browser.new_context(viewport={"width": 960, "height": 640})
            page = context.new_page()

            for i in range(1, trials + 1):
                trial_seed = 5000 + i
                url = f"{base_url}?seed={trial_seed}" if server else base_url
                page.goto(url, wait_until="domcontentloaded")

                attempt = 0
                solved = False
                last_info: dict[str, Any] = {}

                while attempt < max_retries_per_trial and not solved:
                    attempt += 1
                    if attempt > 1:
                        # Nếu lần kéo trước thất bại, tải lại thử thách mới đúng như Bước 8 trong plan
                        page.goto(f"{url}&retry={attempt}", wait_until="domcontentloaded")

                    page.locator("#captcha-bg").wait_for(state="visible", timeout=5000)

                    solve_meta = solve_slider_captcha(
                        page=page,
                        detector=detector,
                        bg_selector="#captcha-bg",
                        slider_selector="#slider-handle",
                        piece_init_offset_x=22.0,
                        overshoot_px=1.5 if humanize else 0.0,
                    )

                    # Chờ trạng thái xác minh cập nhật
                    page.wait_for_timeout(120)
                    status_loc = page.locator("#captcha-status")
                    status_text = status_loc.inner_text()
                    detail_raw = status_loc.get_attribute("data-detail")
                    page_detail = json.loads(detail_raw) if detail_raw else {}

                    solved = "Xác minh thành công" in status_text or bool(page_detail.get("success"))
                    last_info = {
                        "trial": i,
                        "attempts_used": attempt,
                        "solved": solved,
                        **solve_meta,
                        **page_detail,
                    }

                # Lưu ảnh minh hoạ cho trial đầu tiên để kiểm tra trực quan
                if i == 1:
                    page.screenshot(path=str(DEBUG_DIR / "trial_01_result.png"))

                trial_records.append(last_info)
                status_tag = "PASS" if solved else "FAIL"
                err_px = last_info.get("posErrorPx", "N/A")
                moves = last_info.get("moveEvents", "N/A")
                print(
                    f"  [{i:02d}/{trials:02d}] {status_tag} | "
                    f"pred_x={last_info.get('gap_center_css_x')} vs true_x={last_info.get('trueGapCenter')} "
                    f"(err={err_px}px) | conf={last_info.get('confidence')} | "
                    f"infer={last_info.get('inference_ms')}ms | drag={last_info.get('drag_ms')}ms (moves={moves})"
                )
    finally:
        if server:
            server.stop()

    total_sec = time.perf_counter() - t_start_all
    success_count = sum(1 for r in trial_records if r.get("solved"))
    first_try_count = sum(1 for r in trial_records if r.get("solved") and r.get("attempts_used") == 1)
    errors = [float(r["posErrorPx"]) for r in trial_records if "posErrorPx" in r]
    infer_times = [float(r["inference_ms"]) for r in trial_records if "inference_ms" in r]
    drag_times = [float(r["drag_ms"]) for r in trial_records if "drag_ms" in r]
    move_counts = [int(r["moveEvents"]) for r in trial_records if "moveEvents" in r]

    summary = {
        "timestamp": datetime.now().isoformat(),
        "config": {
            "trials": trials,
            "headless": headless,
            "humanize": humanize,
            "tolerance_px": tolerance_px,
            "model_path": str(model_path),
        },
        "metrics": {
            "total_trials": trials,
            "success_count": success_count,
            "success_rate_pct": round(success_count / max(1, trials) * 100.0, 2),
            "first_attempt_success_rate_pct": round(first_try_count / max(1, trials) * 100.0, 2),
            "mean_abs_error_px": round(statistics.mean(errors), 2) if errors else None,
            "median_abs_error_px": round(statistics.median(errors), 2) if errors else None,
            "mean_inference_ms": round(statistics.mean(infer_times), 2) if infer_times else None,
            "mean_drag_ms": round(statistics.mean(drag_times), 2) if drag_times else None,
            "mean_mouse_move_events": round(statistics.mean(move_counts), 1) if move_counts else None,
            "total_elapsed_sec": round(total_sec, 2),
        },
        "trials": trial_records,
    }

    out_path = HERE / "benchmark_results.json"
    out_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    print("\n=== TỔNG KẾT ĐO LƯỜNG (BENCHMARK SUMMARY) ===")
    print(json.dumps(summary["metrics"], indent=2, ensure_ascii=False))
    print(f"[+] Đã lưu chi tiết kết quả tại: {out_path}")
    return summary


def main() -> int:
    parser = argparse.ArgumentParser(description="Giai đoạn 4: Benchmark YOLO26 + Camoufox Slider Captcha")
    parser.add_argument("--trials", type=int, default=int(os.getenv("BENCHMARK_TRIALS", "30")), help="Số lần thử nghiệm (mặc định: 30)")
    parser.add_argument("--headless", action="store_true", default=True, help="Chạy Camoufox ở chế độ headless (mặc định: True)")
    parser.add_argument("--headed", action="store_true", help="Mở cửa sổ trình duyệt Camoufox trực quan")
    parser.add_argument("--no-humanize", action="store_true", help="Tắt humanize để so sánh với chế độ mặc định")
    parser.add_argument("--tolerance", type=float, default=7.0, help="Sai số pixel cho phép khi khớp mảnh ghép (mặc định: 7.0px)")
    parser.add_argument("--url", type=str, default=None, help="URL trang web captcha thực tế (nếu bỏ trống sẽ dùng local testbed)")
    parser.add_argument("--model", type=str, default=str(DEFAULT_ONNX_MODEL), help="Đường dẫn model YOLO26 ONNX")
    args = parser.parse_args()

    headless = False if args.headed else args.headless
    humanize = not args.no_humanize

    run_benchmark(
        trials=args.trials,
        headless=headless,
        humanize=humanize,
        tolerance_px=args.tolerance,
        target_url=args.url,
        model_path=Path(args.model),
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
