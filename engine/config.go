package main

type CalarbotConfig struct {
	Modules     map[string]ModulesConfig `yaml:"modules"`
	TgTokenFile string                   `yaml:"tgTokenFile"`
	SQLitePath  string                   `yaml:"sqlitePath"`
	// SeedChats — разовый посев: чем включённость модулей была до появления
	// админки. Отрабатывает один раз, дальше правда живёт в базе.
	SeedChats []SeedChat `yaml:"seed_chats"`
}

type ModulesConfig struct {
	Url string `yaml:"url"`
}

type SeedChat struct {
	ID      int64    `yaml:"id"`
	Title   string   `yaml:"title"`
	Type    string   `yaml:"type"`
	Modules []string `yaml:"modules"`
}
