FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/linkedin-service ./cmd/linkedin-service

FROM alpine:3.20
COPY --from=build /out/linkedin-service /usr/local/bin/linkedin-service
COPY cmd/linkedin-service/etc /etc/linkedin-service
EXPOSE 8092
CMD ["linkedin-service", "-f", "/etc/linkedin-service/linkedin.docker.yaml"]
