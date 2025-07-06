package common

import (
	"fmt"
	"os"

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
