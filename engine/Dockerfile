FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY common/ ./common/
COPY botModules/ ./botModules/
COPY engine/ ./engine/
RUN go build -o /engine ./engine

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /engine ./engine
CMD ["./engine"]
