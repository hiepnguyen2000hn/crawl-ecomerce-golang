FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/reddit-api-service ./cmd/reddit-api-service

FROM alpine:3.20
COPY --from=build /out/reddit-api-service /usr/local/bin/reddit-api-service
COPY cmd/reddit-api-service/etc /etc/reddit-api-service
EXPOSE 8088
CMD ["reddit-api-service", "-f", "/etc/reddit-api-service/redditapi.docker.yaml"]
