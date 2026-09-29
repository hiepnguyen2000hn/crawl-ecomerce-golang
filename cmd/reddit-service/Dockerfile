FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/reddit-service ./cmd/reddit-service

FROM alpine:3.20
COPY --from=build /out/reddit-service /usr/local/bin/reddit-service
COPY cmd/reddit-service/etc /etc/reddit-service
EXPOSE 8087
CMD ["reddit-service", "-f", "/etc/reddit-service/reddit.docker.yaml"]
