// Vibecoded it because I'm lazy AF

package botModules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockModule implements the BotModule interface for testing
type MockModule struct {
	OrderValue   int
	IsCalledFunc func(*tgbotapi.Message) bool
	AnswerFunc   func(*Payload) (string, error)
}

func (m *MockModule) Order() int {
	return m.OrderValue
}

func (m *MockModule) IsCalled(msg *tgbotapi.Message) bool {
	if m.IsCalledFunc != nil {
		return m.IsCalledFunc(msg)
	}
	return false
}

func (m *MockModule) Answer(payload *Payload) (string, error) {
	if m.AnswerFunc != nil {
		return m.AnswerFunc(payload)
	}
	return "", nil
}

func TestServeModule(t *testing.T) {
	// Create a mock module
	mockModule := &MockModule{
		OrderValue: 42,
		IsCalledFunc: func(msg *tgbotapi.Message) bool {
			if msg == nil {
				return false
			}
			return msg.Text == "call me"
		},
		AnswerFunc: func(payload *Payload) (string, error) {
			if payload == nil || payload.Msg == nil {
				return "", fmt.Errorf("invalid payload")
			}
			if payload.Msg.Text == "error" {
				return "error response", fmt.Errorf("test error")
			}
			return "test answer for: " + payload.Msg.Text, nil
		},
	}

	// Start the server
	addr := "localhost:8081"
	server, errChan := ServeModule(mockModule, addr)

	// Ensure the server is shut down at the end of the test
	defer func() {
		// Create a context with a timeout for shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Shutdown the server
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("Server shutdown error: %v", err)
		}

		// Check for any server errors
		select {
		case err := <-errChan:
			if err != nil && err != http.ErrServerClosed {
				t.Errorf("Server error: %v", err)
			}
		default:
			// No error
		}
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Test the /order endpoint
	t.Run("order endpoint", func(t *testing.T) {
		resp, err := http.Get("http://" + addr + "/order")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", resp.Status)
		}

		var result struct {
			Order int `json:"order"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result.Order != mockModule.OrderValue {
			t.Errorf("Expected order %d, got %d", mockModule.OrderValue, result.Order)
		}
	})

	// Test the /is_called endpoint with a message that should be called
	t.Run("is_called endpoint - called", func(t *testing.T) {
		payload := Payload{
			Msg: &tgbotapi.Message{
				Text: "call me",
			},
		}
		body, _ := json.Marshal(payload)
		resp, err := http.Post("http://"+addr+"/is_called", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", resp.Status)
		}

		var result struct {
			Called bool `json:"called"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if !result.Called {
			t.Errorf("Expected called to be true, got false")
		}
	})

	// Test the /is_called endpoint with a message that should not be called
	t.Run("is_called endpoint - not called", func(t *testing.T) {
		payload := Payload{
			Msg: &tgbotapi.Message{
				Text: "don't call me",
			},
		}
		body, _ := json.Marshal(payload)
		resp, err := http.Post("http://"+addr+"/is_called", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Called bool `json:"called"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result.Called {
			t.Errorf("Expected called to be false, got true")
		}
	})

	// Test the /answer endpoint with a normal message
	t.Run("answer endpoint - normal", func(t *testing.T) {
		payload := Payload{
			Msg: &tgbotapi.Message{
				Text: "hello",
			},
		}
		body, _ := json.Marshal(payload)
		resp, err := http.Post("http://"+addr+"/answer", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", resp.Status)
		}

		var result struct {
			Answer string `json:"answer"`
			Error  string `json:"error,omitempty"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		expectedAnswer := "test answer for: hello"
		if result.Answer != expectedAnswer {
			t.Errorf("Expected answer %q, got %q", expectedAnswer, result.Answer)
		}
		if result.Error != "" {
			t.Errorf("Expected no error, got %q", result.Error)
		}
	})

	// Test the /answer endpoint with an error-triggering message
	t.Run("answer endpoint - error", func(t *testing.T) {
		payload := Payload{
			Msg: &tgbotapi.Message{
				Text: "error",
			},
		}
		body, _ := json.Marshal(payload)
		resp, err := http.Post("http://"+addr+"/answer", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", resp.Status)
		}

		var result struct {
			Answer string `json:"answer"`
			Error  string `json:"error,omitempty"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		expectedAnswer := "error response"
		if result.Answer != expectedAnswer {
			t.Errorf("Expected answer %q, got %q", expectedAnswer, result.Answer)
		}
		if result.Error != "test error" {
			t.Errorf("Expected error %q, got %q", "test error", result.Error)
		}
	})
}
