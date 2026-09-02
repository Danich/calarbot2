package main

import (
	"testing"
)

func TestSeedEnablesListedModules(t *testing.T) {
	b := botWithSettings(t)
	b.BotConfig = &CalarbotConfig{SeedChats: []SeedChat{
		{ID: -1, Title: "болталка", Type: "supergroup", Modules: []string{"skazka", "aiAnswer"}},
	}}

	if err := b.seedSettings(1000); err != nil {
		t.Fatalf("seedSettings: %v", err)
	}

	for _, m := range []string{"skazka", "aiAnswer"} {
		if enabled, err := b.SettingsStore.ModuleEnabled(-1, m); err != nil || !enabled {
			t.Errorf("ModuleEnabled(-1, %s) = %v, %v; want true", m, enabled, err)
		}
	}
	if enabled, _ := b.SettingsStore.ModuleEnabled(-1, "sber"); enabled {
		t.Error("ModuleEnabled(-1, sber) = true; посев не должен включать неперечисленное")
	}

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 || chats[0].Title != "болталка" {
		t.Fatalf("ListChats = %+v; want the seeded chat", chats)
	}
}

// Посев обязан быть разовым: иначе он воскресит модуль, выключенный руками.
func TestSeedRunsOnlyOnce(t *testing.T) {
	b := botWithSettings(t)
	b.BotConfig = &CalarbotConfig{SeedChats: []SeedChat{
		{ID: -1, Type: "group", Modules: []string{"aiAnswer"}},
	}}

	if err := b.seedSettings(1000); err != nil {
		t.Fatalf("seedSettings: %v", err)
	}
	if err := b.SettingsStore.SetModuleEnabled(-1, "aiAnswer", false); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if err := b.seedSettings(2000); err != nil {
		t.Fatalf("seedSettings: %v", err)
	}

	if enabled, _ := b.SettingsStore.ModuleEnabled(-1, "aiAnswer"); enabled {
		t.Error("ModuleEnabled = true after a second seed; посев должен был не сработать")
	}
}

func TestSeedDefaultsMissingTypeToGroup(t *testing.T) {
	b := botWithSettings(t)
	b.BotConfig = &CalarbotConfig{SeedChats: []SeedChat{{ID: -1, Modules: []string{"aiAnswer"}}}}

	if err := b.seedSettings(1000); err != nil {
		t.Fatalf("seedSettings: %v", err)
	}

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 || chats[0].Type != "group" {
		t.Fatalf("chat type = %+v; want group", chats)
	}
}
