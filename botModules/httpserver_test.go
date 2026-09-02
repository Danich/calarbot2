// Vibecoded it because I'm lazy AF

package botModules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockModule implements the BotModule interface for testing
type MockModule struct {
	RegistrationValue Registration
	IsCalledFunc      func(*Payload) bool
	AnswerFunc        func(*Payload) (RichAnswer, error)
}

func (m *MockModule) Register() Registration {
	return m.RegistrationValue
}

func (m *MockModule) IsCalled(payload *Payload) bool {
	if m.IsCalledFunc != nil {
		return m.IsCalledFunc(payload)
	}
	return false
}

func (m *MockModule) Answer(payload *Payload) (RichAnswer, error) {
	if m.AnswerFunc != nil {
		return m.AnswerFunc(payload)
	}
	return RichAnswer{}, nil
}

func TestServeModule(t *testing.T) {
	// Create a mock module
	mockModule := &MockModule{
		RegistrationValue: Registration{Order: 42},
		IsCalledFunc: func(payload *Payload) bool {
			if payload == nil || payload.Msg == nil {
				return false
			}
			return payload.Msg.Text == "call me"
		},
		AnswerFunc: func(payload *Payload) (RichAnswer, error) {
			if payload == nil || payload.Msg == nil {
				return RichAnswer{}, fmt.Errorf("invalid payload")
			}
			if payload.Msg.Text == "error" {
				return RichAnswer{Text: "error response"}, fmt.Errorf("test error")
			}
			return RichAnswer{Text: "test answer for: " + payload.Msg.Text}, nil
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

	// Test the /register endpoint
	t.Run("register endpoint", func(t *testing.T) {
		resp, err := http.Get("http://" + addr + "/register")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", resp.Status)
		}

		var result Registration
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result.Order != mockModule.RegistrationValue.Order {
			t.Errorf("Expected order %d, got %d", mockModule.RegistrationValue.Order, result.Order)
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

type extraSpy struct {
	seen map[string]interface{}
}

func (s *extraSpy) Register() Registration { return Registration{Order: 1} }
func (s *extraSpy) IsCalled(payload *Payload) bool {
	s.seen = payload.Extra
	return true
}
func (s *extraSpy) Answer(payload *Payload) (RichAnswer, error) {
	return RichAnswer{}, nil
}

// Настройки едут к модулю в Extra, и /is_called обязан их донести: без этого
// модуль узнаёт свои настройки только в Answer, а решает-то он раньше.
func TestIsCalledCarriesExtra(t *testing.T) {
	spy := &extraSpy{}
	srv, _ := ServeModule(spy, "127.0.0.1:0")
	defer srv.Close()

	handler := srv.Handler
	body, _ := json.Marshal(Payload{
		Msg:   &tgbotapi.Message{Text: "привет"},
		Extra: map[string]interface{}{"settings": map[string]interface{}{"answer_level": 990.0}},
	})
	req := httptest.NewRequest(http.MethodPost, "/is_called", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	settings, ok := spy.seen["settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("IsCalled saw Extra = %v; want a settings map", spy.seen)
	}
	if settings["answer_level"] != 990.0 {
		t.Errorf("answer_level = %v; want 990", settings["answer_level"])
	}
}
