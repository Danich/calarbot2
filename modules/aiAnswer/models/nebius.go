package models

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// NebiusClient wraps the Nebius AI API for vision (image description).
//
// Генерация картинок отсюда уехала: Nebius её больше не предоставляет — в
// каталоге не осталось ни одной такой модели. Рисованием занимается
// ImageClient, который смотрит на другого провайдера.
type NebiusClient struct {
	apiKey      string
	baseURL     string
	visionModel string
	httpClient  *http.Client
}

// NewNebiusClient creates a new NebiusClient.
func NewNebiusClient(apiKey, baseURL, visionModel string) *NebiusClient {
	return &NebiusClient{
		apiKey:      apiKey,
		baseURL:     baseURL,
		visionModel: visionModel,
		httpClient:  &http.Client{},
	}
}

func (c *NebiusClient) newClient() openai.Client {
	return openai.NewClient(
		option.WithAPIKey(c.apiKey),
		option.WithBaseURL(c.baseURL),
	)
}

// DescribeImage downloads the image from fileURL, base64-encodes it, and sends it
// to the Nebius vision model together with the given prompt.
func (c *NebiusClient) DescribeImage(ctx context.Context, fileURL, prompt string) (string, error) {
	imgBytes, err := c.downloadFile(ctx, fileURL)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(imgBytes)
	dataURL := "data:image/jpeg;base64," + b64

	cl := c.newClient()
	res, err := cl.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.visionModel,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
				openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
					URL: dataURL,
				}),
				openai.TextContentPart(prompt),
			}),
		},
	})
	if err != nil {
		return "", err
	}
	return res.Choices[0].Message.Content, nil
}

func (c *NebiusClient) downloadFile(ctx context.Context, fileURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
