FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY common/ ./common/
COPY notify/ ./notify/
RUN go build -o /notify ./notify

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /notify ./notify
CMD ["./notify"]
