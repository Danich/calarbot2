// Vibecoded it because I'm lazy AF

package main

import (
	"strings"
	"testing"
)

func TestTrimLastPart(t *testing.T) {
	tests := []struct {
		name           string
		messageToSend  string
		lastMessage    string
		expectedResult string
	}{
		{
			name:           "single sentence without period",
			messageToSend:  "Тебе выпало продолжить рассказ: ",
			lastMessage:    "Жил-был король",
			expectedResult: "Тебе выпало продолжить рассказ: ...король",
		},
		{
			name:           "single sentence with period",
			messageToSend:  "Тебе выпало продолжить рассказ: ",
			lastMessage:    "Жил-был король.",
			expectedResult: "Тебе выпало продолжить рассказ: ...король.",
		},
		{
			name:           "multiple sentences",
			messageToSend:  "Тебе выпало продолжить рассказ: ",
			lastMessage:    "Жил-был король. Он правил мудро",
			expectedResult: "Тебе выпало продолжить рассказ:  Он правил мудро",
		},
		{
			name:           "multiple sentences with period at end",
			messageToSend:  "Тебе выпало продолжить рассказ: ",
			lastMessage:    "Жил-был король. Он правил мудро.",
			expectedResult: "Тебе выпало продолжить рассказ:  Он правил мудро.",
		},
		{
			name:           "multiple words in single sentence",
			messageToSend:  "Тебе выпало продолжить рассказ: ",
			lastMessage:    "Жил-был король в далеком королевстве",
			expectedResult: "Тебе выпало продолжить рассказ: ...в далеком королевстве",
		},
		{
			name:           "single word",
			messageToSend:  "Тебе выпало продолжить рассказ: ",
			lastMessage:    "Король",
			expectedResult: "Тебе выпало продолжить рассказ: ...Король",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimLastPart(tt.messageToSend, tt.lastMessage)
			if result != tt.expectedResult {
				t.Errorf("trimLastPart() = %q, want %q", result, tt.expectedResult)
			}
		})
	}
}

func TestNameAnonymousPlayer(t *testing.T) {
	tests := []struct {
		name     string
		username string
		hasAt    bool
	}{
		{
			name:     "empty username",
			username: "",
			hasAt:    false,
		},
		{
			name:     "non-empty username",
			username: "testuser",
			hasAt:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nameAnonymousPlayer(tt.username)

			if tt.hasAt {
				if result != "@"+tt.username {
					t.Errorf("nameAnonymousPlayer(%q) = %q, want %q", tt.username, result, "@"+tt.username)
				}
			} else {
				// For empty username, we should get a random adjective and animal
				// We can't test the exact value, but we can check that it's not empty
				if result == "" {
					t.Errorf("nameAnonymousPlayer(%q) returned empty string", tt.username)
				}

				// Check that it contains a space (adjective + space + animal)
				if !contains(result, " ") {
					t.Errorf("nameAnonymousPlayer(%q) = %q, expected to contain a space", tt.username, result)
				}
			}
		})
	}
}

func TestIsLastTurn(t *testing.T) {
	tests := []struct {
		name        string
		counter     int
		playerCount int
		maxTurns    int
		expected    bool
	}{
		{
			name:        "not last turn - fewer players than max turns",
			counter:     5,
			playerCount: 3,
			maxTurns:    10,
			expected:    false,
		},
		{
			name:        "last turn - fewer players than max turns",
			counter:     10,
			playerCount: 3,
			maxTurns:    10,
			expected:    true,
		},
		{
			name:        "not last turn - more players than max turns",
			counter:     5,
			playerCount: 15,
			maxTurns:    10,
			expected:    false,
		},
		{
			name:        "last turn - more players than max turns",
			counter:     15,
			playerCount: 15,
			maxTurns:    10,
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLastTurn(tt.counter, tt.playerCount, tt.maxTurns)
			if result != tt.expected {
				t.Errorf("isLastTurn(%d, %d, %d) = %v, want %v",
					tt.counter, tt.playerCount, tt.maxTurns, result, tt.expected)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return s != "" && s != substr && s != strings.Replace(s, substr, "", 1)
}
