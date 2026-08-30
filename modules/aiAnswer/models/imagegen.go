package models

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

// ImageClient generates images through any OpenAI-compatible /images/generations
// endpoint.
//
// Отдельно от NebiusClient, потому что рисование уехало к другому провайдеру:
// Nebius убрал генерацию картинок совсем — в его каталоге не осталось ни одной
// такой модели, и запрос отвечал 404. Vision у него при этом живой и остаётся
// на месте.
type ImageClient struct {
	apiKey  string
	baseURL string
	model   string
}

func NewImageClient(apiKey, baseURL, model string) *ImageClient {
	return &ImageClient{apiKey: apiKey, baseURL: baseURL, model: model}
}

// GenerateImage returns the URL of the first generated image.
func (c *ImageClient) GenerateImage(ctx context.Context, prompt string) (string, error) {
	// Пустой ключ или модель — это ненастроенный сервис, а не сбой провайдера.
	// Без этой проверки он превращается в 401 откуда-то из глубины клиента.
	if c.apiKey == "" || c.model == "" {
		return "", fmt.Errorf("image generation is not configured")
	}

	cl := openai.NewClient(
		option.WithAPIKey(c.apiKey),
		option.WithBaseURL(c.baseURL),
	)
	res, err := cl.Images.Generate(ctx, openai.ImageGenerateParams{
		Prompt: prompt,
		Model:  openai.ImageModel(c.model),
		N:      param.NewOpt[int64](1),
	})
	if err != nil {
		return "", err
	}
	if len(res.Data) == 0 {
		return "", fmt.Errorf("no image returned by %s", c.model)
	}
	if res.Data[0].URL == "" {
		return "", fmt.Errorf("empty URL in the response from %s", c.model)
	}
	return res.Data[0].URL, nil
}
