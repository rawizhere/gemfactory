# GemFactory
Telegram bot for tracking music releases and downloading media clips.

## Features
- **Release Schedule**: Fetches release dates, filtered against the active artist list stored in DB.
- **Filters**: View releases by month, filtered by gender (`-f` / `-m`) or user-defined artist lists.
- **Auto-Updates**: Keeps the release calendar up to date in the background.
- **Media Downloader**: Download video clips, GIFs, MP3 audio, and translated subtitles via yt-dlp.
- **Multi-Provider Subtitle Translation**: Subtitle translation using Google Translate, Gemini, or Groq with automatic fallback.
- **Web Admin Panel**: Web UI to manage artists, releases, scraper settings, and translation settings.
- **Dynamic Configuration**: Settings are stored in DB and updated in real-time.

## Commands

### User Commands
- `/start` - Start interaction
- `/help [topic]` - Show available commands
- `/month [month]` - Show releases for month (e.g., `/month april`)
- `/month [month] -f` - Female artists only
- `/month [month] -m` - Male artists only
- `/search [artist]` - Search releases by artist
- `/artists` - Show active artists lists
- `/clip [url] [start] [end] [720p] [hq]` - Download video clip
- `/subs [url] [start] [end] [lang] [720p] [hq] [nollm]` - Cut clip with burned-in subtitles; `nollm` translates via Google Translate, skipping AI providers
- `/gif [url] [start] [end]` - Download clip as GIF

### Admin Commands
- `/add_artist [name] [-f|-m]` - Add artist to list
- `/remove_artist [name]` - Remove artist from list
- `/admin` - Admin panel entry
- `/config [key] [value]` - Set configuration
- `/metrics` - Bot metrics
- `/parse [month/year]` - Parse releases for specific month/year
- `/export` - Export all artists

## Project Structure

```
gemfactory/
├── cmd/bot/                 # Application entry point
├── internal/
│   ├── app/                # App orchestration and router
│   ├── config/             # Configuration management
│   ├── downloader/         # yt-dlp downloader, ffmpeg & translator
│   ├── handlers/           # Telegram command handlers
│   ├── health/             # Health checks
│   ├── keyboard/           # UI builders
│   ├── logger/             # Logging setup
│   ├── middleware/         # Rate limiting & logging
│   ├── model/              # Domain models
│   ├── notify/             # Admin Telegram notifications
│   ├── scraper/            # Data fetchers and parsers
│   ├── service/            # Business logic
│   ├── settings/           # Settings registry and defaults
│   ├── storage/            # Database repositories
│   ├── telegram/           # Bot API client
│   ├── translate/          # Subtitle translation
│   ├── web/                # Admin web panel & REST API
│   └── worker/             # Background jobs
└── migrations/             # SQL migrations
```

## Running

1. Configure `.env` (see `env.example`).
2. Run with Docker:
   ```bash
   docker-compose up -d
   ```

## License
MIT
