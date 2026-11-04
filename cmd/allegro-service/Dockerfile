FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/allegro-service ./cmd/allegro-service

FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    firefox-esr \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/allegro-service /usr/local/bin/allegro-service
COPY cmd/allegro-service/etc /etc/allegro-service
COPY cmd/allegro-service/crawler /app/crawler
COPY scripts/camoufox-yolo-captcha /app/camoufox-yolo-captcha

RUN cd /app/camoufox-yolo-captcha && \
    python3 -m venv .venv && \
    .venv/bin/pip install --no-cache-dir --upgrade pip && \
    .venv/bin/pip install --no-cache-dir opencv-python-headless camoufox onnxruntime python-dotenv

EXPOSE 8089
CMD ["allegro-service", "-f", "/etc/allegro-service/allegro.docker.yaml"]
