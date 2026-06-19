package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type VisionClient interface {
	DescribeImage(ctx context.Context, fileURL, prompt string) (string, error)
}

type VisionHandler struct {
	client    VisionClient
	botToken  string
	BotAPIURL string
}

func NewVisionHandler(client VisionClient, botToken string) *VisionHandler {
	return &VisionHandler{
		client:    client,
		botToken:  botToken,
		BotAPIURL: "https://api.telegram.org",
	}
}

func (h *VisionHandler) Describe(ctx context.Context, msg *tgbotapi.Message) (string, error) {
	if len(msg.Photo) == 0 {
		return "", fmt.Errorf("no photo in message")
	}
	fileID := msg.Photo[len(msg.Photo)-1].FileID

	fileURL, err := getTelegramFileURL(ctx, h.BotAPIURL, h.botToken, fileID)
	if err != nil {
		return "", fmt.Errorf("resolve telegram file: %w", err)
	}

	prompt := msg.Text
	if prompt == "" {
		prompt = msg.Caption
	}
	if prompt == "" {
		prompt = "Describe this image in detail."
	}
	return h.client.DescribeImage(ctx, fileURL, prompt)
}

func getTelegramFileURL(ctx context.Context, botAPIURL, botToken, fileID string) (string, error) {
	apiURL := fmt.Sprintf("%s/bot%s/getFile?file_id=%s",
		botAPIURL, botToken, url.QueryEscape(fileID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.OK {
		return "", fmt.Errorf("getFile failed: %s (file_id %s)", result.Description, fileID)
	}
	return fmt.Sprintf("%s/file/bot%s/%s", botAPIURL, botToken, result.Result.FilePath), nil
}
