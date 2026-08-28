package app

import (
	"testing"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
)

func TestIsGrokFactCheck(t *testing.T) {
	replyMsg := &telego.Message{
		MessageID: 100,
		Text:      "Target text to verify",
	}

	tests := []struct {
		name     string
		msg      *telego.Message
		expected bool
	}{
		{
			name:     "nil message",
			msg:      nil,
			expected: false,
		},
		{
			name: "no reply to message",
			msg: &telego.Message{
				Text: "@grok это правда?",
			},
			expected: false,
		},
		{
			name: "empty text",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "",
			},
			expected: false,
		},
		{
			name: "ru exact match",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok это правда",
			},
			expected: true,
		},
		{
			name: "ru with comma and question mark",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok, это правда?",
			},
			expected: true,
		},
		{
			name: "ru with colon and extra text",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok: это правда или фейк?",
			},
			expected: true,
		},
		{
			name: "ru uppercase",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@GROK ЭТО ПРАВДА?",
			},
			expected: true,
		},
		{
			name: "en exact match",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok is this true",
			},
			expected: true,
		},
		{
			name: "en with question mark and comma",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok, is this true?",
			},
			expected: true,
		},
		{
			name: "en with extra words",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok is this true: please check",
			},
			expected: true,
		},
		{
			name: "unrelated grok mention",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok hello there",
			},
			expected: false,
		},
		{
			name: "just mention",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isGrokFactCheck(tt.msg))
		})
	}
}
