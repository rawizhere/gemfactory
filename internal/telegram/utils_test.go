package telegram

import (
	"strings"
	"testing"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/require"
)

func TestGetUserIdentifier(t *testing.T) {
	require.Equal(t, "unknown", GetUserIdentifier(nil), "want 'unknown' for nil user")

	withUsername := &telego.User{ID: 100, Username: "johndoe", FirstName: "John"}
	require.Equal(t, "johndoe", GetUserIdentifier(withUsername))

	withFullName := &telego.User{ID: 200, FirstName: "Jane", LastName: "Doe"}
	require.Equal(t, "Jane Doe", GetUserIdentifier(withFullName))

	withFirstNameOnly := &telego.User{ID: 300, FirstName: "Alice"}
	require.Equal(t, "Alice", GetUserIdentifier(withFirstNameOnly))

	withIDOnly := &telego.User{ID: 400}
	require.Equal(t, "user_400", GetUserIdentifier(withIDOnly))
}

func TestSplitMessageBasic(t *testing.T) {
	text := "short message"
	require.Equal(t, []string{text}, splitMessage(text, 100))
}

func TestSplitMessageChunksUnderLimit(t *testing.T) {
	lines := make([]string, 0, 50)
	for range 50 {
		lines = append(lines, strings.Repeat("x", 80))
	}
	text := strings.Join(lines, "\n")

	chunks := splitMessage(text, 400)
	require.Greater(t, len(chunks), 1)

	lineCount := 0
	for _, chunk := range chunks {
		require.LessOrEqual(t, len(chunk), 400+len("<b></b>"), "chunk overflow beyond tag rebalance allowance")
		lineCount += strings.Count(chunk, "\n") + 1
	}
	require.Equal(t, len(lines), lineCount, "splitting must preserve all lines")
}

func TestRebalanceHTMLTags(t *testing.T) {
	got := rebalanceHTMLTags([]string{"<b>Hello", "world</b>"})
	require.Equal(t, []string{"<b>Hello</b>", "<b>world</b>"}, got)
}

func TestRebalanceHTMLTagsNested(t *testing.T) {
	got := rebalanceHTMLTags([]string{"<b>x<i>y", "z</i></b>", "tail"})
	require.Equal(t, []string{"<b>x<i>y</i></b>", "<b><i>z</i></b>", "tail"}, got)
}

func TestSplitMessageKeepsValidHTMLAcrossChunks(t *testing.T) {
	var b strings.Builder
	b.WriteString("<b>bold intro\n")
	for range 40 {
		b.WriteString(strings.Repeat("y", 80))
		b.WriteString("\n")
	}
	b.WriteString("bold tail</b>")

	for _, chunk := range splitMessage(b.String(), 400) {
		require.Equal(t, strings.Count(chunk, "<b>"), strings.Count(chunk, "</b>"),
			"every chunk must have balanced <b> tags:\n%s", chunk)
	}
}
