package main

import (
	"log"

	"calarbot2/settings"
)

const seededMetaKey = "seeded"

// seedSettings переносит в базу то, чем включённость модулей была до появления
// админки: enabled_on в конфиге не восстановим автоматически — телеграм не
// отдаёт список чатов бота, а skazka с sber вообще работали везде.
//
// Заодно создаёт строки в chats, иначе сразу после выката панель была бы пуста
// и включать в ней было бы нечего.
//
// Разовый: флаг в settings_meta. Повторный прогон воскресил бы модуль,
// выключенный руками, — то есть ровно то, ради чего админку и делали.
func (b *Bot) seedSettings(now int64) error {
	if _, seeded, err := b.SettingsStore.Meta(seededMetaKey); err != nil {
		return err
	} else if seeded {
		return nil
	}

	for _, seed := range b.BotConfig.SeedChats {
		chatType := seed.Type
		if chatType == "" {
			chatType = "group"
		}
		if err := b.SettingsStore.UpsertChat(settings.Chat{
			ID:        seed.ID,
			Type:      chatType,
			Title:     seed.Title,
			FirstSeen: now,
			LastSeen:  now,
		}); err != nil {
			return err
		}
		for _, module := range seed.Modules {
			if err := b.SettingsStore.SetModuleEnabled(seed.ID, module, true); err != nil {
				return err
			}
		}
		log.Printf("seeded chat %d with modules %v", seed.ID, seed.Modules)
	}

	return b.SettingsStore.SetMeta(seededMetaKey, "1")
}
