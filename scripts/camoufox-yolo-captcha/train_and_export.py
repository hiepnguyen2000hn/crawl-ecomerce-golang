"""
Giai đoạn 1: Chuẩn bị môi trường & Mô hình YOLO26 (Huấn luyện & Xuất ONNX).

Script này hỗ trợ 2 chế độ:
1. Nếu bạn chưa có dataset: Tự động sinh bộ dataset Slider Captcha có gán nhãn
   chuẩn YOLO (bounding box cho khe hở 'gap'), huấn luyện mô hình `yolo26n.pt`
   và xuất ra định dạng ONNX (`models/yolo26n_captcha.onnx`).
2. Nếu bạn đã có dataset riêng: Truyền `--data path/to/your_captcha_data.yaml`
   để huấn luyện và xuất ONNX theo đúng kế hoạch.

Cách chạy:
    ./.venv/bin/python train_and_export.py --epochs 15 --samples 160
"""
import argparse
import math
import random
import shutil
from pathlib import Path

import cv2
import numpy as np
from ultralytics import YOLO

HERE = Path(__file__).parent
DATASET_DIR = HERE / "dataset"
MODELS_DIR = HERE / "models"


def draw_jigsaw_shape(mask: np.ndarray, x: int, y: int, size: int = 44) -> tuple[int, int, int, int]:
    """Vẽ hình mảnh ghép jigsaw (hình vuông kèm các tai tròn lồi/lõm) lên mask."""
    r = size // 5
    # Thân chính mảnh ghép
    cv2.rectangle(mask, (x, y), (x + size, y + size), 255, -1)
    # Tai lồi bên phải và phía trên (giống jigsaw thực tế của DataDome/Geetest)
    cv2.circle(mask, (x + size, y + size // 2), r, 255, -1)
    cv2.circle(mask, (x + size // 2, y), r, 255, -1)

    x1 = max(0, x)
    y1 = max(0, y - r)
    x2 = min(mask.shape[1], x + size + r)
    y2 = min(mask.shape[0], y + size)
    return x1, y1, x2, y2


def generate_synthetic_background(width: int = 320, height: int = 160, rng: random.Random = random) -> np.ndarray:
    """Tạo ảnh nền giàu chi tiết (gradient + họa tiết hình học + nhiễu nhẹ) giống ảnh nền captcha."""
    img = np.zeros((height, width, 3), dtype=np.uint8)

    # Gradient nền ngẫu nhiên
    c1 = [rng.randint(40, 220) for _ in range(3)]
    c2 = [rng.randint(40, 220) for _ in range(3)]
    for row in range(height):
        alpha = row / max(1, height - 1)
        color = [int(c1[i] * (1 - alpha) + c2[i] * alpha) for i in range(3)]
        img[row, :] = color

    # Thêm các khối họa tiết nền để mô hình học phân biệt khe hở thật với vật thể nền
    for _ in range(rng.randint(8, 16)):
        shape_type = rng.choice(["circle", "rect", "line"])
        color = tuple(rng.randint(20, 240) for _ in range(3))
        if shape_type == "circle":
            cx, cy = rng.randint(0, width), rng.randint(0, height)
            rad = rng.randint(8, 35)
            cv2.circle(img, (cx, cy), rad, color, -1)
        elif shape_type == "rect":
            rx1, ry1 = rng.randint(0, width - 30), rng.randint(0, height - 30)
            rx2, ry2 = rx1 + rng.randint(15, 50), ry1 + rng.randint(15, 50)
            cv2.rectangle(img, (rx1, ry1), (rx2, ry2), color, -1)
        else:
            pt1 = (rng.randint(0, width), rng.randint(0, height))
            pt2 = (rng.randint(0, width), rng.randint(0, height))
            cv2.line(img, pt1, pt2, color, rng.randint(1, 3))

    img = cv2.GaussianBlur(img, (3, 3), 0)
    return img


def render_captcha_sample(
    width: int = 320,
    height: int = 160,
    piece_size: int = 44,
    gap_x: int | None = None,
    gap_y: int | None = None,
    draw_floating_piece: bool = True,
    seed: int | None = None,
) -> tuple[np.ndarray, tuple[int, int, int, int], int]:
    """
    Tạo một mẫu ảnh Slider Captcha cùng tọa độ bounding box của khe hở (gap).

    Trả về:
        (image_bgr, (x1, y1, x2, y2), gap_center_x)
    """
    rng = random.Random(seed)
    img = generate_synthetic_background(width, height, rng)

    if gap_x is None:
        gap_x = rng.randint(110, width - piece_size - 20)
    if gap_y is None:
        gap_y = rng.randint(25, height - piece_size - 15)

    mask = np.zeros((height, width), dtype=np.uint8)
    x1, y1, x2, y2 = draw_jigsaw_shape(mask, gap_x, gap_y, piece_size)

    # Làm tối vùng khe hở (inner shadow) và vẽ viền trắng mờ giống captcha thật
    darkened = (img.astype(np.float32) * 0.32).astype(np.uint8)
    img = np.where(mask[..., None] > 0, darkened, img)

    contours, _ = cv2.findContours(mask, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE)
    # Viền bóng đổ + viền sáng của khe hở
    cv2.drawContours(img, contours, -1, (20, 20, 20), 2)
    cv2.drawContours(img, contours, -1, (235, 235, 235), 1)

    # Tuỳ chọn vẽ mảnh ghép (piece) nằm ở rìa trái ở cùng độ cao y (distractor thực tế)
    if draw_floating_piece:
        piece_x = rng.randint(8, 22)
        p_mask = np.zeros((height, width), dtype=np.uint8)
        draw_jigsaw_shape(p_mask, piece_x, gap_y, piece_size)
        p_contours, _ = cv2.findContours(p_mask, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE)
        # Mảnh ghép bên trái sáng hơn và có viền vàng/trắng nổi bật
        brightened = np.clip(img.astype(np.float32) * 1.15 + 20, 0, 255).astype(np.uint8)
        img = np.where(p_mask[..., None] > 0, brightened, img)
        cv2.drawContours(img, p_contours, -1, (255, 240, 150), 2)

    gap_center_x = int(round((x1 + x2) / 2))
    return img, (x1, y1, x2, y2), gap_center_x


def build_synthetic_dataset(out_dir: Path, num_train: int = 160, num_val: int = 40) -> Path:
    """Sinh dataset chuẩn Ultralytics YOLO và trả về đường dẫn file YAML."""
    if out_dir.exists():
        shutil.rmtree(out_dir)

    for split, count in [("train", num_train), ("val", num_val)]:
        img_dir = out_dir / "images" / split
        lbl_dir = out_dir / "labels" / split
        img_dir.mkdir(parents=True, exist_ok=True)
        lbl_dir.mkdir(parents=True, exist_ok=True)

        for idx in range(count):
            seed = (1000 if split == "train" else 9000) + idx
            img, (x1, y1, x2, y2), _ = render_captcha_sample(
                width=320,
                height=160,
                draw_floating_piece=(idx % 3 != 0),
                seed=seed,
            )
            h, w = img.shape[:2]
            xc = ((x1 + x2) / 2.0) / w
            yc = ((y1 + y2) / 2.0) / h
            bw = (x2 - x1) / w
            bh = (y2 - y1) / h

            img_path = img_dir / f"captcha_{idx:04d}.jpg"
            lbl_path = lbl_dir / f"captcha_{idx:04d}.txt"
            cv2.imwrite(str(img_path), img)
            lbl_path.write_text(f"0 {xc:.6f} {yc:.6f} {bw:.6f} {bh:.6f}\n", encoding="utf-8")

    yaml_path = out_dir / "captcha_data.yaml"
    yaml_content = (
        f"path: {out_dir.resolve()}\n"
        f"train: images/train\n"
        f"val: images/val\n"
        f"names:\n"
        f"  0: gap\n"
    )
    yaml_path.write_text(yaml_content, encoding="utf-8")
    return yaml_path


def train_and_export(
    data_yaml: Path,
    base_model: str = "yolo26n.pt",
    epochs: int = 15,
    imgsz: int = 320,
    output_onnx: Path = MODELS_DIR / "yolo26n_captcha.onnx",
) -> Path:
    """
    Thực hiện Giai đoạn 1.1:
      1. Huấn luyện YOLO26 (`yolo26n.pt`) trên dataset captcha.
      2. Xuất sang định dạng ONNX để suy luận nhanh và nhẹ.
    """
    MODELS_DIR.mkdir(parents=True, exist_ok=True)

    print(f"[+] Giai đoạn 1.1: Khởi tạo mô hình {base_model}...")
    model = YOLO(base_model)

    print(f"[+] Bắt đầu huấn luyện trên {data_yaml} (epochs={epochs}, imgsz={imgsz})...")
    model.train(
        data=str(data_yaml),
        epochs=epochs,
        imgsz=imgsz,
        batch=16,
        project=str(HERE / "runs"),
        name="captcha_yolo26n",
        exist_ok=True,
        verbose=False,
    )

    best_pt = HERE / "runs" / "captcha_yolo26n" / "weights" / "best.pt"
    trained_model = YOLO(str(best_pt)) if best_pt.exists() else model

    # Lưu bản .pt và xuất sang .onnx
    pt_target = output_onnx.with_suffix(".pt")
    if best_pt.exists():
        shutil.copy2(best_pt, pt_target)

    print("[+] Xuất mô hình sang định dạng ONNX...")
    exported_path = Path(trained_model.export(format="onnx", imgsz=imgsz))
    if exported_path.resolve() != output_onnx.resolve():
        shutil.copy2(exported_path, output_onnx)

    print(f"[+] Hoàn tất Giai đoạn 1! Model ONNX đã lưu tại: {output_onnx}")
    return output_onnx


def main() -> int:
    parser = argparse.ArgumentParser(description="Giai đoạn 1: Huấn luyện & Xuất YOLO26 sang ONNX")
    parser.add_argument("--data", type=str, default=None, help="Đường dẫn tới captcha_data.yaml (nếu để trống sẽ tự sinh dataset mẫu)")
    parser.add_argument("--model", type=str, default="yolo26n.pt", help="Mô hình gốc Ultralytics (mặc định: yolo26n.pt)")
    parser.add_argument("--epochs", type=int, default=15, help="Số epochs huấn luyện (mặc định: 15)")
    parser.add_argument("--imgsz", type=int, default=320, help="Kích thước ảnh huấn luyện & xuất ONNX (mặc định: 320)")
    parser.add_argument("--samples", type=int, default=160, help="Số lượng mẫu train tự sinh khi không truyền --data")
    parser.add_argument("--out", type=str, default=str(MODELS_DIR / "yolo26n_captcha.onnx"), help="Đường dẫn file ONNX đầu ra")
    args = parser.parse_args()

    if args.data:
        yaml_path = Path(args.data)
    else:
        print(f"[+] Đang sinh bộ dataset Slider Captcha mẫu ({args.samples} train / {max(20, args.samples // 4)} val)...")
        yaml_path = build_synthetic_dataset(DATASET_DIR, num_train=args.samples, num_val=max(20, args.samples // 4))
        print(f"[+] Đã tạo dataset tại: {yaml_path}")

    train_and_export(
        data_yaml=yaml_path,
        base_model=args.model,
        epochs=args.epochs,
        imgsz=args.imgsz,
        output_onnx=Path(args.out),
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
