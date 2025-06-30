# Build stage for engine
FROM golang:1.22-alpine AS engine-builder
WORKDIR /app
COPY . .
RUN cd engine && go build -o /engine

# Build stage for simpleReply module
FROM golang:1.22-alpine AS simplereply-builder
WORKDIR /app
COPY . .
RUN cd modules/simpleReply && go build -o /simpleReply

# Build stage for skazka module
FROM golang:1.22-alpine AS skazka-builder
WORKDIR /app
COPY . .
RUN cd modules/skazka && go build -o /skazka

# Final image
FROM alpine:3.19
WORKDIR /app
COPY --from=engine-builder /engine /engine
COPY --from=simplereply-builder /simpleReply /simpleReply
COPY --from=skazka-builder /skazka /skazka

# Expose default ports (can be overridden in docker-compose)
EXPOSE 8080

# Entrypoint is set in docker-compose for each service
