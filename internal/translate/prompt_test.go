package translate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gemfactory/migrations"
)

// The web panel edits the DB copy; the code default must match the latest seeded prompt so clearing the field changes nothing.
func TestDefaultPromptMatchesLatestMigration(t *testing.T) {
	sql, err := migrations.FS.ReadFile("000017_translation_prompt_youtube.up.sql")
	require.NoError(t, err)
	require.Contains(t, string(sql), strings.ReplaceAll(DefaultTranslationPrompt, "'", "''"))
}
