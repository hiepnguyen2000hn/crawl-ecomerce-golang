FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /pipiads-service ./cmd/pipiads-service

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /pipiads-service /app/pipiads-service
COPY cmd/pipiads-service/etc/pipiads.docker.yaml /app/etc/pipiads.yaml

EXPOSE 8094
CMD ["/app/pipiads-service", "-f", "/app/etc/pipiads.yaml"]
