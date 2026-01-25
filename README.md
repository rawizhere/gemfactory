# GemFactory
Telegram bot for tracking music releases from kpopofficial.com.

## Features
- **Release Schedule**: Fetches release dates from external sources.
- **Filters**: View releases by month, filtered by gender (`-f` / `-m`) or user-defined artist lists.
- **Auto-Updates**: Keeps the release calendar up to date in the background.
- **Dynamic Configuration**: settings are stored in DB and updated in real-time via Config Watcher.

## Commands

### User Commands
- `/start` - Start interaction
- `/help` - Show available commands
- `/month [month]` - Show releases for month (e.g., `/month april`)
- `/month [month] -f` - Female artists only
- `/month [month] -m` - Male artists only
- `/search [artist]` - Search releases by artist
- `/artists` - Show active artists lists
- `/playlist` - Playlist information

### Admin Commands
- `/add_artist [name] [-f|-m]` - Add artist to list
- `/remove_artist [name]` - Remove artist from list
- `/config [key] [value]` - Set configuration
- `/config_list` - Show configuration
- `/config_reset` - Reset configuration
- `/reload_playlist` - Reload playlist
- `/parse [month/year]` - Parse releases for specific month/year
- `/export` - Export all artists

## Project Structure

```
gemfactory/
├── cmd/bot/                 # Application entry point
├── internal/
│   ├── app/                # App orchestration
│   ├── config/             # Configuration management
│   ├── handlers/           # Telegram command handlers
│   ├── health/             # Health checks
│   ├── keyboard/           # UI builders
│   ├── middleware/         # Rate limiting & logging
│   ├── model/              # Domain models
│   ├── scraper/            # Data fetchers
│   ├── service/            # Business logic
│   ├── spotify/            # Spotify integration
│   ├── storage/            # Database repositories
│   ├── telegram/           # Bot API client
│   ├── validator/          # Input validation
│   └── worker/             # Background jobs
├── migrations/             # SQL migrations
└── pkg/                    # Shared packages
```

## Running

1. Configure `.env` (see `env.example`).
2. Run with Docker:
   ```bash
   docker-compose up -d
   ```

## License
MIT
