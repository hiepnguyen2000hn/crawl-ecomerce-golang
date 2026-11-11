FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/kalodata-service ./cmd/kalodata-service

FROM alpine:3.20
COPY --from=build /out/kalodata-service /usr/local/bin/kalodata-service
COPY cmd/kalodata-service/etc /etc/kalodata-service
EXPOSE 8093
CMD ["kalodata-service", "-f", "/etc/kalodata-service/kalodata.docker.yaml"]
