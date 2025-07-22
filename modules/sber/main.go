package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
)

// Module implements the BotModule interface
type Module struct {
	order      int
	sberifyURL string
}

// Order returns the module's priority order
func (m Module) Order() int { return m.order }

// IsCalled checks if this module should handle the message
func (m Module) IsCalled(msg *tgbotapi.Message) bool {
	if msg == nil || msg.Text == "" {
		return false
	}

	// Check if the message starts with /sber
	return strings.HasPrefix(msg.Text, "/sber")
}

// Answer processes the message and returns a response
func (m Module) Answer(payload *botModules.Payload) (string, error) {
	msg := payload.Msg

	// Extract the text after the /sber command
	text := extractTextAfterCommand(msg.Text, "/sber")

	// If there's no text or we're replying to a message, use the replied message text
	if text == "" && msg.ReplyToMessage != nil {
		text = msg.ReplyToMessage.Text
	}

	// If there's still no text, return an error message
	if text == "" {
		return "Пожалуйста, укажите текст после команды /sber или ответьте на сообщение", nil
	}

	// Call the sberify service
	result, err := callSberifyService(m.sberifyURL, text)
	if err != nil {
		return fmt.Sprintf("Ошибка при обработке текста: %v", err), nil
	}

	return result, nil
}

// extractTextAfterCommand extracts the text after a command
func extractTextAfterCommand(text, command string) string {
	if text == command {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(text, command))
}

// callSberifyService calls the sberify service to process the text
func callSberifyService(url, text string) (string, error) {
	// Prepare the request payload
	payload := map[string]string{"text": text}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Send the request to the sberify service
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Parse the response
	var result struct {
		Result string `json:"result"`
		Error  string `json:"error,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Error != "" {
		return "", fmt.Errorf(result.Error)
	}

	return result.Result, nil
}

func main() {
	// Get the module order from environment variable or use default
	order := 500
	if orderEnv := os.Getenv("MODULE_ORDER"); orderEnv != "" {
		if v, err := strconv.Atoi(orderEnv); err == nil {
			order = v
		}
	}

	// Get the sberify service URL from environment variable or use default
	sberifyURL := "http://sberify-service:5000/sberify"
	if urlEnv := os.Getenv("SBERIFY_URL"); urlEnv != "" {
		sberifyURL = urlEnv
	}

	// Get the module port from environment variable or use default
	port := "8080"
	if portEnv := os.Getenv("MODULE_PORT"); portEnv != "" {
		port = portEnv
	}

	// Create and serve the module
	module := Module{
		order:      order,
		sberifyURL: sberifyURL,
	}

	// Start the server and handle graceful shutdown
	if err := botModules.RunModuleServer(module, ":"+port, 0); err != nil {
		fmt.Println(err)
	}
}
