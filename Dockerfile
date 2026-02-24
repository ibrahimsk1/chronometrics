FROM golang:1.22-alpine AS base
WORKDIR /app

FROM base AS builder
COPY . .
RUN go build -o bin/ingestor ./cmd/ingestor && go build -o bin/consumer ./cmd/consumer

FROM alpine:3.18 AS ingestor
COPY --from=builder /app/bin/ingestor /bin/ingestor
ENTRYPOINT ["/bin/ingestor"]

FROM alpine:3.18 AS consumer
COPY --from=builder /app/bin/consumer /bin/consumer
ENTRYPOINT ["/bin/consumer"]

