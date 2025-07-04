package common

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Contains(slice []int64, value int64) bool {
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
