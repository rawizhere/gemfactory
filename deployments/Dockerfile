FROM golang:1.27-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o gemfactory ./cmd/bot

FROM alpine:3.21
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    postgresql-client \
    freetype \
    ffmpeg \
    python3 \
    libass \
    nodejs \
    unzip \
    curl

# Lato font for burned-in subtitles (not packaged in alpine 3.21).
RUN mkdir -p /usr/share/fonts/lato \
    && curl -fsSL -o /usr/share/fonts/lato/Lato-Regular.ttf \
      https://github.com/google/fonts/raw/main/ofl/lato/Lato-Regular.ttf \
    && fc-cache -f >/dev/null

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