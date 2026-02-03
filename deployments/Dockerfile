FROM golang:1.25.6-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gemfactory cmd/bot/main.go

FROM alpine:3.21
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    tzdata \
    postgresql-client \
    ttf-freefont

WORKDIR /app

COPY --from=builder /app/gemfactory ./
COPY --from=builder /app/migrations ./migrations
COPY deployments/start.sh ./start.sh

RUN mkdir -p /app/data /app/logs && \
    chmod +x /app/start.sh /app/gemfactory && \
    chmod -R 777 /app/logs /app/data

WORKDIR /app

EXPOSE 8080

CMD ["/app/start.sh"]