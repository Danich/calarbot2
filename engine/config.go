package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const configPath = "/calarbot.yaml"

type CalarbotConfig struct {
	Modules     map[string]ModulesConfig `yaml:"modules"`
	TgTokenFile string                   `yaml:"tgTokenFile"`
}

type ModulesConfig struct {
	//Name      string `yaml:"name"`
	Url       string `yaml:"url"`
	EnabledOn []int  `yaml:"enabled_on,omitempty"`
}

func (c *CalarbotConfig) Read() error {
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	return yaml.Unmarshal(configFile, c)
}
