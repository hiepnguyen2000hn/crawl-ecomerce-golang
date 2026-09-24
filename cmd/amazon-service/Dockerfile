FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/amazon-service ./cmd/amazon-service

FROM alpine:3.20
COPY --from=build /out/amazon-service /usr/local/bin/amazon-service
COPY cmd/amazon-service/etc /etc/amazon-service
EXPOSE 8085
CMD ["amazon-service", "-f", "/etc/amazon-service/amazon.docker.yaml"]
