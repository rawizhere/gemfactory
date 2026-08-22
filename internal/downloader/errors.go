package downloader

import (
	"strings"
)

// FriendlyError converts raw internal error text into a short actionable
// Telegram message.
func FriendlyError(raw string) string {
	switch {
	case strings.Contains(raw, "Sign in to confirm you're not a bot"),
		strings.Contains(raw, "Sign in to confirm you are not a bot"):
		return "🔒 YouTube требует авторизацию с этого сервера. Добавьте куки youtube.com во вкладке ytdlp веб-панели и повторите."

	case strings.Contains(raw, "No supported JavaScript runtime"):
		return "⚙️ В окружении нет deno (нужен yt-dlp для YouTube). Пересоберите образ: docker compose build."

	case strings.Contains(raw, "слишком длинный"):
		return raw

	case strings.Contains(raw, "The page needs to be reloaded"):
		return "🍪 Куки устарели или были отозваны YouTube. Сделайте свежий экспорт из Cookie-Editor и сохраните во вкладке ytdlp."

	case strings.Contains(raw, "Private video"),
		strings.Contains(raw, "members-only"),
		strings.Contains(raw, "Join this channel"):
		return "🔒 Видео недоступно без подписки/доступа. Нужны куки аккаунта с доступом."

	case strings.Contains(raw, "Video unavailable"),
		strings.Contains(raw, "removed by the uploader"):
		return "🗑 Видео недоступно или удалено."

	default:
		msg := strings.TrimSpace(raw)
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return "❌ Ошибка: " + msg
	}
}
