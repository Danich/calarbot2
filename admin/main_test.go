package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// Реестр модулей у панели тот же, что у движка: тот же файл, тот же ключ.
// Собрали бота без sber — строки нет, и в панели он не появится.
func TestModuleURLsComeFromTheEngineConfig(t *testing.T) {
	const raw = `
tgTokenFile: /.tgtoken
sqlitePath: /data/calarbot.db
modules:
  aiAnswer:
    url: "http://aiAnswer:8080"
  skazka:
    url: "http://skazka:8080"
`
	var c AdminConfig
	if err := yaml.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	urls := moduleURLs(c)

	if len(urls) != 2 {
		t.Fatalf("moduleURLs = %v; want two entries", urls)
	}
	if urls["aiAnswer"] != "http://aiAnswer:8080" {
		t.Errorf("aiAnswer url = %q", urls["aiAnswer"])
	}
	if c.SQLitePath != "/data/calarbot.db" || c.TgTokenFile != "/.tgtoken" {
		t.Errorf("config = %+v; want the db path and token file read", c)
	}
}
