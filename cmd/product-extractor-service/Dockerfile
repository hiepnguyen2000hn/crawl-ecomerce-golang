FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/product-extractor-service ./cmd/product-extractor-service

FROM alpine:3.20
COPY --from=build /out/product-extractor-service /usr/local/bin/product-extractor-service
COPY cmd/product-extractor-service/etc /etc/product-extractor-service
CMD ["product-extractor-service", "-f", "/etc/product-extractor-service/extractor.docker.yaml"]
