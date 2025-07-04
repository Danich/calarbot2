package main

type CalarbotConfig struct {
	Modules     map[string]ModulesConfig `yaml:"modules"`
	TgTokenFile string                   `yaml:"tgTokenFile"`
}

type ModulesConfig struct {
	//Name      string `yaml:"name"`
	Url       string  `yaml:"url"`
	EnabledOn []int64 `yaml:"enabled_on,omitempty"`
}
