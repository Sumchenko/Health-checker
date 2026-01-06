FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /checker ./cmd/checker/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /checker .


HEALTHCHECK --interval=30s --timeout=3s \
    CMD ps aux | grep '[c]hecker' || exit 1

CMD ["./checker"]