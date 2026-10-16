FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/apify-token-service ./cmd/apify-token-service

FROM alpine:3.20
COPY --from=build /out/apify-token-service /usr/local/bin/apify-token-service
COPY cmd/apify-token-service/etc /etc/apify-token-service
EXPOSE 8089
CMD ["apify-token-service", "-f", "/etc/apify-token-service/tokens.docker.yaml"]
