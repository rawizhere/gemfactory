package downloader

import (
	"strings"
	"testing"
)

func TestFriendlyError(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "bot check",
			raw:  "ERROR: [youtube] abc: Sign in to confirm you're not a bot. Use --cookies",
			want: "🔒 YouTube требует авторизацию с этого сервера. Добавьте куки youtube.com во вкладке ytdlp веб-панели и повторите.",
		},
		{
			name: "no js runtime",
			raw:  "WARNING: No supported JavaScript runtime could be found. Only deno is enabled by default",
			want: "⚙️ В окружении нет deno (нужен yt-dlp для YouTube). Пересоберите образ: docker compose build.",
		},
		{
			name: "private video",
			raw:  "ERROR: Private video. Sign in if you've been granted access",
			want: "🔒 Видео недоступно без подписки/доступа. Нужны куки аккаунта с доступом.",
		},
		{
			name: "generic truncated",
			raw:  strings.Repeat("x", 500),
			want: "❌ Ошибка: " + strings.Repeat("x", 300) + "…",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FriendlyError(c.raw); got != c.want {
				t.Errorf("FriendlyError() mismatch:\n got %q\nwant %q", got, c.want)
			}
		})
	}
}
