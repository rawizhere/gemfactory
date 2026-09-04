package app

import (
	"testing"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"

	"gemfactory/internal/handlers"
)

func TestDetectGrokMode(t *testing.T) {
	replyMsg := &telego.Message{
		MessageID: 100,
		Text:      "Target text to verify",
	}

	factcheck := handlers.GrokFactCheck
	retell := handlers.GrokRetell
	opinion := handlers.GrokOpinion

	tests := []struct {
		name     string
		msg      *telego.Message
		expected *handlers.GrokMode
	}{
		{
			name:     "nil message",
			msg:      nil,
			expected: nil,
		},
		{
			name: "no reply to message",
			msg: &telego.Message{
				Text: "@grok это правда?",
			},
			expected: nil,
		},
		{
			name: "empty text",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "",
			},
			expected: nil,
		},
		{
			name: "ru exact match",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok это правда",
			},
			expected: &factcheck,
		},
		{
			name: "ru with comma and question mark",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok, это правда?",
			},
			expected: &factcheck,
		},
		{
			name: "ru with colon and extra text",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok: это правда или фейк?",
			},
			expected: &factcheck,
		},
		{
			name: "ru uppercase",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@GROK ЭТО ПРАВДА",
			},
			expected: &factcheck,
		},
		{
			name: "en exact match",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok is this true",
			},
			expected: &factcheck,
		},
		{
			name: "en with question mark and comma",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok, is this true?",
			},
			expected: &factcheck,
		},
		{
			name: "en with extra words",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok is this true: please check",
			},
			expected: &factcheck,
		},
		{
			name: "retell bare",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok перескажи",
			},
			expected: &retell,
		},
		{
			name: "retell with comma and question mark",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok, перескажи?",
			},
			expected: &retell,
		},
		{
			name: "retell with extra words",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok перескажи коротко",
			},
			expected: &retell,
		},
		{
			name: "retell polite form",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok перескажите",
			},
			expected: &retell,
		},
		{
			name: "retell other verb form",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok пересказать",
			},
			expected: &retell,
		},
		{
			name: "retell with exclamation mark",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok перескажи!",
			},
			expected: &retell,
		},
		{
			name: "retell en",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok summarize",
			},
			expected: &retell,
		},
		{
			name: "opinion bare",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok мнение",
			},
			expected: &opinion,
		},
		{
			name: "opinion with question mark",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok мнение?",
			},
			expected: &opinion,
		},
		{
			name: "opinion plural",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok мнения?",
			},
			expected: &opinion,
		},
		{
			name: "opinion en",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok opinion?",
			},
			expected: &opinion,
		},
		{
			name: "factcheck with url in the command",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok это правда https://example.com/watch?v=abc",
			},
			expected: &factcheck,
		},
		{
			name: "url alone is not a command",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok https://example.com/watch?v=abc",
			},
			expected: nil,
		},
		{
			name: "factcheck only matches right after the mention",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok мнение, это правда?",
			},
			expected: &opinion,
		},
		{
			name: "unrelated grok mention",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok hello there",
			},
			expected: nil,
		},
		{
			name: "just mention",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok",
			},
			expected: nil,
		},
		{
			name: "near-miss retell word",
			msg: &telego.Message{
				ReplyToMessage: replyMsg,
				Text:           "@grok перестань",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, ok := detectGrokMode(tt.msg)
			if tt.expected == nil {
				assert.False(t, ok)
				return
			}
			assert.True(t, ok)
			assert.Equal(t, *tt.expected, mode)
		})
	}
}
