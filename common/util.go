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

// ExtractMentions extracts all mentions from a Telegram message,
// including mentions in photo/video captions.
// It correctly handles UTF-16 code units used by Telegram for entity offsets.
func ExtractMentions(msg *tgbotapi.Message) []string {
	if msg == nil {
		return nil
	}
	var mentions []string
	if len(msg.Entities) > 0 {
		mentions = append(mentions, extractMentionsFromEntities(msg.Text, msg.Entities)...)
	}
	if len(msg.CaptionEntities) > 0 {
		mentions = append(mentions, extractMentionsFromEntities(msg.Caption, msg.CaptionEntities)...)
	}
	return mentions
}

func extractMentionsFromEntities(text string, entities []tgbotapi.MessageEntity) []string {
	utf16Text := Utf16CodeUnits(text)
	var mentions []string
	for _, entity := range entities {
		if entity.Offset >= 0 && entity.Length > 0 && entity.Offset+entity.Length <= len(utf16Text) {
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
