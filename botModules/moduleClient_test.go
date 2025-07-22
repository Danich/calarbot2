// Vibecoded it because I'm lazy AF

package botModules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestModuleClientOrder(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse interface{}
		serverStatus   int
		expected       int
	}{
		{
			name:           "successful response",
			serverResponse: map[string]int{"order": 5},
			serverStatus:   http.StatusOK,
			expected:       5,
		},
		{
			name:           "error response",
			serverResponse: map[string]string{"error": "something went wrong"},
			serverStatus:   http.StatusInternalServerError,
			expected:       9999,
		},
		{
			name:           "invalid response format",
			serverResponse: "not a json",
			serverStatus:   http.StatusOK,
			expected:       9999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check if the request path is correct
				if r.URL.Path != "/order" {
					t.Errorf("Expected request to '/order', got: %s", r.URL.Path)
				}

				// Check if the request method is GET
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET request, got: %s", r.Method)
				}

				// Set response status
				w.WriteHeader(tt.serverStatus)

				// Write response
				if resp, ok := tt.serverResponse.(string); ok {
					_, _ = fmt.Fprint(w, resp)
				} else {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			// Create client with test server URL
			client := &ModuleClient{
				BaseURL: server.URL,
			}

			// Call the method
			result := client.Order()

			// Check the result
			if result != tt.expected {
				t.Errorf("Order() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestModuleClientIsCalled(t *testing.T) {
	tests := []struct {
		name           string
		payload        *Payload
		serverResponse interface{}
		serverStatus   int
		expectedCalled bool
		expectError    bool
	}{
		{
			name: "successful response - called",
			payload: &Payload{
				Msg: &tgbotapi.Message{
					Text: "test message",
				},
				Extra: map[string]interface{}{
					"key": "value",
				},
			},
			serverResponse: map[string]bool{"called": true},
			serverStatus:   http.StatusOK,
			expectedCalled: true,
			expectError:    false,
		},
		{
			name: "successful response - not called",
			payload: &Payload{
				Msg: &tgbotapi.Message{
					Text: "test message",
				},
			},
			serverResponse: map[string]bool{"called": false},
			serverStatus:   http.StatusOK,
			expectedCalled: false,
			expectError:    false,
		},
		{
			name: "invalid response format",
			payload: &Payload{
				Msg: &tgbotapi.Message{
					Text: "test message",
				},
			},
			serverResponse: "not a json",
			serverStatus:   http.StatusOK,
			expectedCalled: false,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check if the request path is correct
				if r.URL.Path != "/is_called" {
					t.Errorf("Expected request to '/is_called', got: %s", r.URL.Path)
				}

				// Check if the request method is POST
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got: %s", r.Method)
				}

				// Check content type
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type: application/json, got: %s", r.Header.Get("Content-Type"))
				}

				// Set response status
				w.WriteHeader(tt.serverStatus)

				// Write response
				if resp, ok := tt.serverResponse.(string); ok {
					_, _ = fmt.Fprint(w, resp)
				} else {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			// Create client with test server URL
			client := &ModuleClient{
				BaseURL: server.URL,
			}

			// Call the method
			called, err := client.IsCalled(tt.payload)

			// Check for error
			if (err != nil) != tt.expectError {
				t.Errorf("IsCalled() error = %v, expectError %v", err, tt.expectError)
				return
			}

			// If no error, check the result
			if !tt.expectError && called != tt.expectedCalled {
				t.Errorf("IsCalled() = %v, want %v", called, tt.expectedCalled)
			}
		})
	}
}

func TestModuleClientAnswer(t *testing.T) {
	tests := []struct {
		name           string
		payload        *Payload
		serverResponse interface{}
		serverStatus   int
		expectedAnswer string
		expectError    bool
	}{
		{
			name: "successful response",
			payload: &Payload{
				Msg: &tgbotapi.Message{
					Text: "test message",
				},
				Extra: map[string]interface{}{
					"key": "value",
				},
			},
			serverResponse: map[string]string{"answer": "This is the answer"},
			serverStatus:   http.StatusOK,
			expectedAnswer: "This is the answer",
			expectError:    false,
		},
		{
			name: "response with error field",
			payload: &Payload{
				Msg: &tgbotapi.Message{
					Text: "test message",
				},
			},
			serverResponse: map[string]string{
				"answer": "Partial answer",
				"error":  "Something went wrong",
			},
			serverStatus:   http.StatusOK,
			expectedAnswer: "Partial answer",
			expectError:    true,
		},
		{
			name: "server error",
			payload: &Payload{
				Msg: &tgbotapi.Message{
					Text: "test message",
				},
			},
			serverResponse: map[string]string{"error": "internal server error"},
			serverStatus:   http.StatusInternalServerError,
			expectedAnswer: "",
			expectError:    true,
		},
		{
			name: "invalid response format",
			payload: &Payload{
				Msg: &tgbotapi.Message{
					Text: "test message",
				},
			},
			serverResponse: "not a json",
			serverStatus:   http.StatusOK,
			expectedAnswer: "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check if the request path is correct
				if r.URL.Path != "/answer" {
					t.Errorf("Expected request to '/answer', got: %s", r.URL.Path)
				}

				// Check if the request method is POST
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got: %s", r.Method)
				}

				// Check content type
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type: application/json, got: %s", r.Header.Get("Content-Type"))
				}

				// Set response status
				w.WriteHeader(tt.serverStatus)

				// Write response
				if resp, ok := tt.serverResponse.(string); ok {
					_, _ = fmt.Fprint(w, resp)
				} else {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			// Create client with test server URL
			client := &ModuleClient{
				BaseURL: server.URL,
			}

			// Call the method
			answer, err := client.Answer(tt.payload)

			// Check for error
			if (err != nil) != tt.expectError {
				t.Errorf("Answer() error = %v, expectError %v", err, tt.expectError)
				return
			}

			// Check the result
			if answer != tt.expectedAnswer {
				t.Errorf("Answer() = %q, want %q", answer, tt.expectedAnswer)
			}
		})
	}
}
