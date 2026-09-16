FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ai-service ./cmd/ai-service

FROM alpine:3.20
COPY --from=build /out/ai-service /usr/local/bin/ai-service
COPY cmd/ai-service/etc /etc/ai-service
CMD ["ai-service", "-f", "/etc/ai-service/ai.docker.yaml"]
