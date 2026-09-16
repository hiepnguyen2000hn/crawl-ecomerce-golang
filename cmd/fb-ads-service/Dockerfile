FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/fb-ads-service ./cmd/fb-ads-service

FROM alpine:3.20
COPY --from=build /out/fb-ads-service /usr/local/bin/fb-ads-service
COPY cmd/fb-ads-service/etc /etc/fb-ads-service
EXPOSE 8082
CMD ["fb-ads-service", "-f", "/etc/fb-ads-service/fbads.docker.yaml"]
