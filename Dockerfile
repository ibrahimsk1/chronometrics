FROM golang:1.22-alpine AS base
WORKDIR /app

FROM base AS builder
COPY . .
RUN go build -o bin/ingestor ./cmd/ingestor

FROM alpine:3.18 AS ingestor
COPY --from=builder /app/bin/ingestor /bin/ingestor
COPY --from=builder /app/migrations /app/migrations
WORKDIR /app
ENTRYPOINT ["/bin/ingestor"]

