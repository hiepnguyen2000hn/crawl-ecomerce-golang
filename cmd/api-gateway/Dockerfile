FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api-gateway ./cmd/api-gateway

FROM alpine:3.20
COPY --from=build /out/api-gateway /usr/local/bin/api-gateway
COPY cmd/api-gateway/etc /etc/api-gateway
EXPOSE 8888
CMD ["api-gateway", "-f", "/etc/api-gateway/gateway-api.docker.yaml"]
