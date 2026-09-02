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

func TestModuleClientRegister(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/register" {
			t.Errorf("path = %q; want /register", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Registration{
			Order: 100,
			Label: "AI-ответ",
			Fields: []Field{{
				Key: "answer_level", Label: "Вес", Type: FieldNumber, Default: 990,
			}},
		})
	}))
	defer srv.Close()

	c := &ModuleClient{BaseURL: srv.URL}
	reg, err := c.Register()
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if reg.Order != 100 || reg.Label != "AI-ответ" {
		t.Errorf("Registration = %+v; want order 100, label AI-ответ", reg)
	}
	if len(reg.Fields) != 1 || reg.Fields[0].Key != "answer_level" {
		t.Errorf("Fields = %+v; want one answer_level field", reg.Fields)
	}
}

// Модуль лежит — он всё равно должен оказаться последним в очереди, а не
// первым. Это поведение было у Order() и его нельзя потерять.
func TestModuleClientRegisterSinksUnreachableModule(t *testing.T) {
	c := &ModuleClient{BaseURL: "http://127.0.0.1:1"}

	reg, err := c.Register()
	if err == nil {
		t.Error("Register on an unreachable module returned nil error")
	}
	if reg.Order != 9999 {
		t.Errorf("Order = %d; want 9999", reg.Order)
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
			if answer.Text != tt.expectedAnswer {
				t.Errorf("Answer().Text = %q, want %q", answer.Text, tt.expectedAnswer)
			}
		})
	}
}

// photoModule отвечает картинкой в байтах и ничем больше.
type photoModule struct{ img []byte }

func (m photoModule) Register() Registration              { return Registration{Order: 1} }
func (m photoModule) IsCalled(*Payload) bool              { return true }
func (m photoModule) Answer(*Payload) (RichAnswer, error) { return RichAnswer{Photo: m.img}, nil }

// Байты картинки едут от модуля к движку через JSON, и это единственное место,
// где они могут молча потеряться: до этой ручки генератор отдаёт base64, после
// неё телеграм ждёт байты. Тест гоняет настоящий сервер модуля и настоящего
// клиента, а не мок с одной стороны.
func TestModuleClientCarriesPhotoBytes(t *testing.T) {
	img := []byte{0x89, 'P', 'N', 'G', 0x00, 0xFF, 0xFE, 0x01}

	server, errChan := ServeModule(photoModule{img: img}, "127.0.0.1:0")
	defer server.Close()
	select {
	case err := <-errChan:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("сервер модуля: %v", err)
		}
	default:
	}

	ts := httptest.NewServer(server.Handler)
	defer ts.Close()

	client := &ModuleClient{BaseURL: ts.URL}
	answer, err := client.Answer(&Payload{Msg: &tgbotapi.Message{}})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if string(answer.Photo) != string(img) {
		t.Fatalf("картинка приехала как %v, а отправлялась %v", answer.Photo, img)
	}
}

// Модуль без картинки не должен присылать пустое поле, которое движок примет
// за картинку нулевой длины.
func TestModuleClientLeavesPhotoEmptyWhenThereIsNone(t *testing.T) {
	server, _ := ServeModule(photoModule{}, "127.0.0.1:0")
	defer server.Close()

	ts := httptest.NewServer(server.Handler)
	defer ts.Close()

	answer, err := (&ModuleClient{BaseURL: ts.URL}).Answer(&Payload{Msg: &tgbotapi.Message{}})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if len(answer.Photo) != 0 {
		t.Fatalf("ожидалась пустая картинка, приехало %d байт", len(answer.Photo))
	}
}
