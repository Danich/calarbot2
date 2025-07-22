FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY common/ ./common/
COPY botModules/ ./botModules/
COPY modules/simpleReply/ ./modules/simpleReply/
RUN go build -o /simpleReply ./modules/simpleReply

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /simpleReply ./simpleReply
CMD ["./simpleReply"]
