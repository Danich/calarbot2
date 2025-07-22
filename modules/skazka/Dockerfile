FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY common/ ./common/
COPY botModules/ ./botModules/
COPY modules/skazka/ ./modules/skazka/
RUN go build -o /skazka ./modules/skazka

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /skazka ./skazka
CMD ["./skazka"]
