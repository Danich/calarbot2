FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY common/ ./common/
COPY botModules/ ./botModules/
COPY modules/sber/ ./modules/sber/
RUN go build -o /sber ./modules/sber

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /sber ./sber
CMD ["./sber"]
