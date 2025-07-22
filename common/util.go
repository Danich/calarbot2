package common

import (
	"fmt"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gopkg.in/yaml.v3"
)

// Contains checks if a slice contains a specific value.
// It works with any comparable type (int, string, etc.).
func Contains[T comparable](slice []T, value T) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func ReadConfig(configPath string, c interface{}) error {
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	return yaml.Unmarshal(configFile, c)
}

// ExtractMentions extracts all mentions from a Telegram message.
// It correctly handles UTF-16 code units used by Telegram for entity offsets.
func ExtractMentions(msg *tgbotapi.Message) []string {
	var mentions []string
	if msg == nil || msg.Entities == nil {
		return mentions
	}

	// Convert the message text to UTF-16 code units
	utf16Text := Utf16CodeUnits(msg.Text)

	for _, entity := range msg.Entities {
		// Check if offset and length are within bounds
		if entity.Offset >= 0 && entity.Length > 0 && entity.Offset+entity.Length <= len(utf16Text) {
			// Extract the mention using UTF-16 indices
			mention := Utf16ToString(utf16Text[entity.Offset : entity.Offset+entity.Length])
			mentions = append(mentions, mention)
		}
	}
	return mentions
}

// Utf16CodeUnits converts a UTF-8 string to a slice of UTF-16 code units
func Utf16CodeUnits(s string) []uint16 {
	result := make([]uint16, 0, len(s))
	for _, r := range s {
		if r <= 0xFFFF {
			result = append(result, uint16(r))
		} else {
			// Encode as surrogate pair
			r -= 0x10000
			result = append(result, 0xD800+uint16(r>>10), 0xDC00+uint16(r&0x3FF))
		}
	}
	return result
}

// Utf16ToString converts a slice of UTF-16 code units back to a UTF-8 string
func Utf16ToString(u []uint16) string {
	var result []rune
	for i := 0; i < len(u); i++ {
		if i+1 < len(u) && u[i] >= 0xD800 && u[i] <= 0xDBFF && u[i+1] >= 0xDC00 && u[i+1] <= 0xDFFF {
			// Surrogate pair
			r := (rune(u[i]-0xD800)<<10 | rune(u[i+1]-0xDC00)) + 0x10000
			result = append(result, r)
			i++
		} else {
			result = append(result, rune(u[i]))
		}
	}
	return string(result)
}
