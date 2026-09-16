FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/trend-service ./cmd/trend-service

FROM alpine:3.20
COPY --from=build /out/trend-service /usr/local/bin/trend-service
COPY cmd/trend-service/etc /etc/trend-service
EXPOSE 8081
CMD ["trend-service", "-f", "/etc/trend-service/trend.docker.yaml"]
