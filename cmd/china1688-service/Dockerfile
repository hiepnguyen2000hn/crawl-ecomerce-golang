FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/china1688-service ./cmd/china1688-service

FROM alpine:3.20
COPY --from=build /out/china1688-service /usr/local/bin/china1688-service
COPY cmd/china1688-service/etc /etc/china1688-service
EXPOSE 8086
CMD ["china1688-service", "-f", "/etc/china1688-service/china1688.docker.yaml"]
