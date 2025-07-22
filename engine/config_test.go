// Vibecoded it because I'm lazy AF

package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCalarbotConfig(t *testing.T) {
	// Test case for parsing a valid YAML configuration
	t.Run("valid config", func(t *testing.T) {
		// Create a temporary YAML file
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")

		// Sample configuration
		configYAML := `
modules:
  module1:
    url: "http://localhost:8080"
    enabled_on: [123456789, 987654321]
  module2:
    url: "http://localhost:8081"
tgTokenFile: "/path/to/token"
`

		// Write the YAML to the temporary file
		err := os.WriteFile(configPath, []byte(configYAML), 0644)
		if err != nil {
			t.Fatalf("Failed to write temporary config file: %v", err)
		}

		// Read the file and unmarshal it
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config file: %v", err)
		}

		var config CalarbotConfig
		err = yaml.Unmarshal(data, &config)
		if err != nil {
			t.Fatalf("Failed to unmarshal config: %v", err)
		}

		// Verify the configuration was parsed correctly
		if len(config.Modules) != 2 {
			t.Errorf("Expected 2 modules, got %d", len(config.Modules))
		}

		// Check module1
		module1, exists := config.Modules["module1"]
		if !exists {
			t.Errorf("Expected module1 to exist in config")
		} else {
			if module1.Url != "http://localhost:8080" {
				t.Errorf("Expected module1 URL to be 'http://localhost:8080', got '%s'", module1.Url)
			}

			if len(module1.EnabledOn) != 2 {
				t.Errorf("Expected module1 to be enabled on 2 chats, got %d", len(module1.EnabledOn))
			} else {
				if module1.EnabledOn[0] != 123456789 {
					t.Errorf("Expected first enabled_on value to be 123456789, got %d", module1.EnabledOn[0])
				}
				if module1.EnabledOn[1] != 987654321 {
					t.Errorf("Expected second enabled_on value to be 987654321, got %d", module1.EnabledOn[1])
				}
			}
		}

		// Check module2
		module2, exists := config.Modules["module2"]
		if !exists {
			t.Errorf("Expected module2 to exist in config")
		} else {
			if module2.Url != "http://localhost:8081" {
				t.Errorf("Expected module2 URL to be 'http://localhost:8081', got '%s'", module2.Url)
			}

			if module2.EnabledOn != nil && len(module2.EnabledOn) != 0 {
				t.Errorf("Expected module2 to have no enabled_on values, got %v", module2.EnabledOn)
			}
		}

		// Check tgTokenFile
		if config.TgTokenFile != "/path/to/token" {
			t.Errorf("Expected tgTokenFile to be '/path/to/token', got '%s'", config.TgTokenFile)
		}
	})

	// Test case for empty configuration
	t.Run("empty config", func(t *testing.T) {
		var config CalarbotConfig
		err := yaml.Unmarshal([]byte(""), &config)
		if err != nil {
			t.Fatalf("Failed to unmarshal empty config: %v", err)
		}

		if config.Modules != nil && len(config.Modules) != 0 {
			t.Errorf("Expected empty modules map, got %v", config.Modules)
		}

		if config.TgTokenFile != "" {
			t.Errorf("Expected empty tgTokenFile, got '%s'", config.TgTokenFile)
		}
	})

	// Test case for partial configuration
	t.Run("partial config", func(t *testing.T) {
		configYAML := `
modules:
  module1:
    url: "http://localhost:8080"
`

		var config CalarbotConfig
		err := yaml.Unmarshal([]byte(configYAML), &config)
		if err != nil {
			t.Fatalf("Failed to unmarshal partial config: %v", err)
		}

		if len(config.Modules) != 1 {
			t.Errorf("Expected 1 module, got %d", len(config.Modules))
		}

		module1, exists := config.Modules["module1"]
		if !exists {
			t.Errorf("Expected module1 to exist in config")
		} else {
			if module1.Url != "http://localhost:8080" {
				t.Errorf("Expected module1 URL to be 'http://localhost:8080', got '%s'", module1.Url)
			}

			if module1.EnabledOn != nil && len(module1.EnabledOn) != 0 {
				t.Errorf("Expected module1 to have no enabled_on values, got %v", module1.EnabledOn)
			}
		}

		if config.TgTokenFile != "" {
			t.Errorf("Expected empty tgTokenFile, got '%s'", config.TgTokenFile)
		}
	})
}
