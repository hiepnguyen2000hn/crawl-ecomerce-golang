FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/kalodata-service ./cmd/kalodata-service

FROM python:3.12-slim

# Camoufox downloads its own patched Firefox; install the shared libraries that
# binary links against. Keyword search logs in via a real browser session.
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      libgtk-3-0 libdbus-glib-1-2 libasound2 libx11-xcb1 libxtst6 \
      libxrandr2 libxcomposite1 libxdamage1 libxfixes3 libxcursor1 \
      libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 libcups2 libpango-1.0-0 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# search.py only needs Camoufox (no captcha solver / torch), so keep it light.
RUN python3 -m venv /app/venv && \
    /app/venv/bin/pip install --no-cache-dir --upgrade pip && \
    /app/venv/bin/pip install --no-cache-dir "camoufox[geoip]"

ENV PATH="/app/venv/bin:$PATH"
RUN /app/venv/bin/python -m camoufox fetch

COPY cmd/kalodata-service/crawler /app/cmd/kalodata-service/crawler
COPY --from=build /out/kalodata-service /usr/local/bin/kalodata-service
COPY cmd/kalodata-service/etc /etc/kalodata-service

EXPOSE 8093
CMD ["kalodata-service", "-f", "/etc/kalodata-service/kalodata.docker.yaml"]
