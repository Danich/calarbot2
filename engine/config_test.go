// Vibecoded it because I'm lazy AF

package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCalarbotConfig(t *testing.T) {
	const raw = `
tgTokenFile: /.tgtoken
sqlitePath: /data/calarbot.db
modules:
  aiAnswer:
    url: "http://aiAnswer:8080"
seed_chats:
  - id: -386946235
    title: "тестовый"
    type: group
    modules: [skazka, aiAnswer]
`
	var config CalarbotConfig
	if err := yaml.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if config.SQLitePath != "/data/calarbot.db" {
		t.Errorf("SQLitePath = %q; want /data/calarbot.db", config.SQLitePath)
	}
	if config.Modules["aiAnswer"].Url != "http://aiAnswer:8080" {
		t.Errorf("aiAnswer url = %q", config.Modules["aiAnswer"].Url)
	}
	if len(config.SeedChats) != 1 {
		t.Fatalf("SeedChats = %+v; want one entry", config.SeedChats)
	}
	seed := config.SeedChats[0]
	if seed.ID != -386946235 || seed.Type != "group" {
		t.Errorf("seed = %+v; want id -386946235, type group", seed)
	}
	if len(seed.Modules) != 2 || seed.Modules[0] != "skazka" {
		t.Errorf("seed modules = %v; want [skazka aiAnswer]", seed.Modules)
	}
}
