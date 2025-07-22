FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY common/ ./common/
COPY botModules/ ./botModules/
COPY modules/aiAnswer/ ./modules/aiAnswer/
RUN go build -o /aiAnswer ./modules/aiAnswer

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /aiAnswer ./aiAnswer
CMD ["./aiAnswer"]
