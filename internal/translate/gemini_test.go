package translate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestRetryDelay(t *testing.T) {
	withInfo := genai.APIError{Details: []map[string]any{
		{"@type": "type.googleapis.com/google.rpc.QuotaFailure"},
		{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "17s"},
	}}
	require.Equal(t, 17*time.Second, retryDelay(withInfo))
	require.Equal(t, 5*time.Second, retryDelay(genai.APIError{}), "default without RetryInfo")
}

func TestKeyInvalid(t *testing.T) {
	badKey := genai.APIError{Code: 400, Details: []map[string]any{{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "API_KEY_INVALID"}}}
	require.True(t, keyInvalid(badKey))
	require.True(t, keyInvalid(genai.APIError{Code: 403}))
	require.False(t, keyInvalid(genai.APIError{Code: 400}), "plain 400 is a rejected option, not a bad key")
}

func TestDropOption(t *testing.T) {
	cfg := baseConfig("system", 0.2)
	cfg.ResponseMIMEType = "application/json"
	cfg.ResponseSchema = &genai.Schema{Type: genai.TypeArray}

	require.True(t, dropOption(cfg))
	require.Nil(t, cfg.ThinkingConfig, "thinking is dropped first")
	require.NotNil(t, cfg.ResponseSchema)

	require.True(t, dropOption(cfg))
	require.Nil(t, cfg.ResponseSchema)
	require.Empty(t, cfg.ResponseMIMEType)

	require.True(t, dropOption(cfg))
	require.Nil(t, cfg.SafetySettings)

	require.False(t, dropOption(cfg), "nothing left to drop")
}

func TestBuildTranslateUserMsg(t *testing.T) {
	msg := buildTranslateUserMsg([]string{"a", "b"}, "ru", "en", "Title")
	require.Contains(t, msg, "JSON array of exactly 2 strings")
	require.Contains(t, msg, "Source language: English (en)")
	require.Contains(t, msg, `Video context / Title: "Title"`)
	require.Contains(t, msg, "1. a\n2. b")
}
