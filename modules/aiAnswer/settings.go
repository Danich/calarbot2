package main

import "calarbot2/botModules"

// Настройки чата приезжают от движка в Extra — модуль их не хранит и в базу за
// ними не ходит. Полную карту собирает движок, так что фолбэки здесь нужны
// только на случай вызова в обход него: тесты, локальный запуск, старый движок.
func settingsOf(payload *botModules.Payload) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	s, _ := payload.Extra["settings"].(map[string]any)
	if s == nil {
		return map[string]any{}
	}
	return s
}

// intSetting понимает и int, и float64: по проводу настройки едут json'ом, а
// там всякое число — float64.
func intSetting(s map[string]any, key string, fallback int) int {
	switch v := s[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return fallback
	}
}

func stringSetting(s map[string]any, key, fallback string) string {
	if v, ok := s[key].(string); ok && v != "" {
		return v
	}
	return fallback
}
