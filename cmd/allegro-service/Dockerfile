FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/allegro-service ./cmd/allegro-service

FROM python:3.12-slim

# Camoufox downloads its own patched Firefox rather than using a distro build,
# so install the shared libraries that binary links against -- not firefox-esr.
# libgl1 + libglib2.0-0: ultralytics pulls full opencv-python which needs
# libGL.so.1 (import cv2 fails without it even though opencv-python-headless
# is also installed).
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      libgl1 libglib2.0-0 \
      libgtk-3-0 libdbus-glib-1-2 libasound2 libx11-xcb1 libxtst6 \
      libxrandr2 libxcomposite1 libxdamage1 libxfixes3 libxcursor1 \
      libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 libcups2 libpango-1.0-0 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# captcha_solver.py imports ultralytics, which pulls ~3GB of nvidia-cuda wheels
# from PyPI by default. Install CPU-only torch from the pytorch index first so
# the ultralytics install below finds the dependency already satisfied.
RUN python3 -m venv /app/venv && \
    /app/venv/bin/pip install --no-cache-dir --upgrade pip && \
    /app/venv/bin/pip install --no-cache-dir \
      --index-url https://download.pytorch.org/whl/cpu torch torchvision && \
    /app/venv/bin/pip install --no-cache-dir \
      "camoufox[geoip]" ultralytics opencv-python-headless onnxruntime python-dotenv

ENV PATH="/app/venv/bin:$PATH"

# Pull the patched Firefox at build time so the first crawl does not stall on
# an ~80MB download.
RUN /app/venv/bin/python -m camoufox fetch

# fetch.py resolves REPO_ROOT as Path(__file__).parents[3] and imports
# captcha_solver from REPO_ROOT/scripts/camoufox-yolo-captcha. Both paths below
# are load-bearing: flattening either one breaks that import.
COPY cmd/allegro-service/crawler /app/cmd/allegro-service/crawler
COPY scripts/camoufox-yolo-captcha /app/scripts/camoufox-yolo-captcha

COPY --from=build /out/allegro-service /usr/local/bin/allegro-service
COPY cmd/allegro-service/etc /etc/allegro-service

EXPOSE 8089
CMD ["allegro-service", "-f", "/etc/allegro-service/allegro.docker.yaml"]
