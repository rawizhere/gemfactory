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
    font-dejavu \
    nodejs \
    unzip \
    curl

# Lato for burned subtitles; monochrome Noto Emoji (font-noto-emoji CBDT is unusable by libass).
# Fonts pinned to google/fonts@ec62651 to keep builds reproducible.
RUN mkdir -p /usr/share/fonts/lato \
    && curl -fsSL -o /usr/share/fonts/lato/Lato-Regular.ttf \
      https://github.com/google/fonts/raw/ec626514f79f831f1ab848a82114a0ce7e2d6372/ofl/lato/Lato-Regular.ttf \
    && curl -fsSL -o /usr/share/fonts/lato/NotoEmoji.ttf \
      "https://github.com/google/fonts/raw/ec626514f79f831f1ab848a82114a0ce7e2d6372/ofl/notoemoji/NotoEmoji%5Bwght%5D.ttf" \
    && fc-cache -f >/dev/null \
    && addgroup -S app && adduser -S app -G app

# bgutil POT plugin; YouTube gates subtitle tracks behind PO tokens now.
ARG BGUTIL_POT_VERSION=1.3.2
RUN mkdir -p /etc/yt-dlp/plugins \
    && curl -fsSL -o /etc/yt-dlp/plugins/bgutil-ytdlp-pot-provider.zip \
      "https://github.com/Brainicism/bgutil-ytdlp-pot-provider/releases/download/${BGUTIL_POT_VERSION}/bgutil-ytdlp-pot-provider.zip"

WORKDIR /app

COPY --from=builder --chown=app:app /app/gemfactory ./
COPY --chown=app:app deployments/start.sh ./start.sh

RUN chmod +x ./start.sh ./gemfactory \
    && mkdir -p /app/data /app/logs \
    && chown -R app:app /app/data /app/logs

USER app

WORKDIR /app

EXPOSE 8080

CMD ["/app/start.sh"]