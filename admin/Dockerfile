FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY common/ ./common/
COPY botModules/ ./botModules/
COPY settings/ ./settings/
COPY admin/ ./admin/
RUN go build -o /admin ./admin

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /admin ./admin
CMD ["./admin"]
