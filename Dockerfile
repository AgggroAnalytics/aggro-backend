# Build
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server \
 && CGO_ENABLED=0 go build -o /temporal-worker ./cmd/temporal-worker

# Run (default: HTTP API; override command to /temporal-worker for finalize activity worker)
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /server ./server
COPY --from=builder /temporal-worker ./temporal-worker
EXPOSE 8080
CMD ["./server"]
