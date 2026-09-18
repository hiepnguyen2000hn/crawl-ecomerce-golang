FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/niche-research-service ./cmd/niche-research-service

FROM alpine:3.20
COPY --from=build /out/niche-research-service /usr/local/bin/niche-research-service
COPY cmd/niche-research-service/etc /etc/niche-research-service
EXPOSE 8084
CMD ["niche-research-service", "-f", "/etc/niche-research-service/niche.docker.yaml"]
