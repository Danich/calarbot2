package common

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestContainsInt64(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int64
		value    int64
		expected bool
	}{
		{
			name:     "value exists in slice",
			slice:    []int64{1, 2, 3, 4, 5},
			value:    3,
			expected: true,
		},
		{
			name:     "value does not exist in slice",
			slice:    []int64{1, 2, 3, 4, 5},
			value:    6,
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []int64{},
			value:    1,
			expected: false,
		},
		{
			name:     "value at beginning of slice",
			slice:    []int64{1, 2, 3, 4, 5},
			value:    1,
			expected: true,
		},
		{
			name:     "value at end of slice",
			slice:    []int64{1, 2, 3, 4, 5},
			value:    5,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(tt.slice, tt.value)
			if result != tt.expected {
				t.Errorf("Contains(%v, %d) = %v, want %v", tt.slice, tt.value, result, tt.expected)
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		value    string
		expected bool
	}{
		{
			name:     "value exists in slice",
			slice:    []string{"apple", "banana", "cherry", "date", "elderberry"},
			value:    "cherry",
			expected: true,
		},
		{
			name:     "value does not exist in slice",
			slice:    []string{"apple", "banana", "cherry", "date", "elderberry"},
			value:    "fig",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			value:    "apple",
			expected: false,
		},
		{
			name:     "value at beginning of slice",
			slice:    []string{"apple", "banana", "cherry", "date", "elderberry"},
			value:    "apple",
			expected: true,
		},
		{
			name:     "value at end of slice",
			slice:    []string{"apple", "banana", "cherry", "date", "elderberry"},
			value:    "elderberry",
			expected: true,
		},
		{
			name:     "case sensitivity",
			slice:    []string{"Apple", "Banana", "Cherry"},
			value:    "apple",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(tt.slice, tt.value)
			if result != tt.expected {
				t.Errorf("Contains(%v, %q) = %v, want %v", tt.slice, tt.value, result, tt.expected)
			}
		})
	}
}

func TestUtf16CodeUnits(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []uint16
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []uint16{},
		},
		{
			name:     "ASCII string",
			input:    "Hello",
			expected: []uint16{72, 101, 108, 108, 111},
		},
		{
			name:     "string with non-ASCII characters",
			input:    "Привет",
			expected: []uint16{1055, 1088, 1080, 1074, 1077, 1090},
		},
		{
			name:     "string with emoji (surrogate pair)",
			input:    "😀",
			expected: []uint16{0xD83D, 0xDE00},
		},
		{
			name:     "mixed string",
			input:    "Hello 😀 Привет",
			expected: []uint16{72, 101, 108, 108, 111, 32, 0xD83D, 0xDE00, 32, 1055, 1088, 1080, 1074, 1077, 1090},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Utf16CodeUnits(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Utf16CodeUnits(%q) length = %d, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Utf16CodeUnits(%q)[%d] = %d, want %d", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestUtf16ToString(t *testing.T) {
	tests := []struct {
		name     string
		input    []uint16
		expected string
	}{
		{
			name:     "empty slice",
			input:    []uint16{},
			expected: "",
		},
		{
			name:     "ASCII string",
			input:    []uint16{72, 101, 108, 108, 111},
			expected: "Hello",
		},
		{
			name:     "string with non-ASCII characters",
			input:    []uint16{1055, 1088, 1080, 1074, 1077, 1090},
			expected: "Привет",
		},
		{
			name:     "string with emoji (surrogate pair)",
			input:    []uint16{0xD83D, 0xDE00},
			expected: "😀",
		},
		{
			name:     "mixed string",
			input:    []uint16{72, 101, 108, 108, 111, 32, 0xD83D, 0xDE00, 32, 1055, 1088, 1080, 1074, 1077, 1090},
			expected: "Hello 😀 Привет",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Utf16ToString(tt.input)
			if result != tt.expected {
				t.Errorf("Utf16ToString(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		name     string
		msg      *tgbotapi.Message
		expected []string
	}{
		{
			name:     "nil message",
			msg:      nil,
			expected: []string{},
		},
		{
			name: "message with nil entities",
			msg: &tgbotapi.Message{
				Text:     "Hello @user",
				Entities: nil,
			},
			expected: []string{},
		},
		{
			name: "message with no entities",
			msg: &tgbotapi.Message{
				Text:     "Hello @user",
				Entities: []tgbotapi.MessageEntity{},
			},
			expected: []string{},
		},
		{
			name: "message with one mention (ASCII)",
			msg: &tgbotapi.Message{
				Text: "Hello @user",
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 6,
						Length: 5,
					},
				},
			},
			expected: []string{"@user"},
		},
		{
			name: "message with multiple mentions (ASCII)",
			msg: &tgbotapi.Message{
				Text: "Hello @user1 and @user2",
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 6,
						Length: 6,
					},
					{
						Type:   "mention",
						Offset: 17,
						Length: 6,
					},
				},
			},
			expected: []string{"@user1", "@user2"},
		},
		{
			name: "message with Cyrillic text and mention",
			msg: &tgbotapi.Message{
				Text: "Привет @user",
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 7,
						Length: 5,
					},
				},
			},
			expected: []string{"@user"},
		},
		{
			name: "message with emoji and mention",
			msg: &tgbotapi.Message{
				Text: "😀 @user",
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 3,
						Length: 5,
					},
				},
			},
			expected: []string{"@user"},
		},
		{
			name: "message with invalid entity (offset out of bounds)",
			msg: &tgbotapi.Message{
				Text: "Hello",
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 10,
						Length: 5,
					},
				},
			},
			expected: []string{},
		},
		{
			name: "message with invalid entity (length out of bounds)",
			msg: &tgbotapi.Message{
				Text: "Hello @user",
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 6,
						Length: 10,
					},
				},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractMentions(tt.msg)
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractMentions() returned %d mentions, want %d", len(result), len(tt.expected))
				return
			}
			for i, mention := range result {
				if mention != tt.expected[i] {
					t.Errorf("ExtractMentions()[%d] = %q, want %q", i, mention, tt.expected[i])
				}
			}
		})
	}
}
