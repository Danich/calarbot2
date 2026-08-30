package models

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

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

// GenerateImage returns the bytes of the first generated image.
//
// Байты, а не ссылка: OpenRouter отдаёт картинку только в b64_json, поля url в
// его ответе нет вовсе. Первая версия читала url, всегда получала пустую строку
// и выбрасывала успешно нарисованные картинки.
func (c *ImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	// Пустой ключ или модель — это ненастроенный сервис, а не сбой провайдера.
	// Без этой проверки он превращается в 401 откуда-то из глубины клиента.
	if c.apiKey == "" || c.model == "" {
		return nil, fmt.Errorf("image generation is not configured")
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
		return nil, err
	}
	if len(res.Data) == 0 {
		return nil, fmt.Errorf("no image returned by %s", c.model)
	}

	if b64 := res.Data[0].B64JSON; b64 != "" {
		img, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("decoding the image from %s: %w", c.model, err)
		}
		return img, nil
	}
	// Ветка на случай провайдера, который отдаёт ссылку. У OpenRouter это не
	// так, но эндпоинт общий и хоронить её незачем.
	if url := res.Data[0].URL; url != "" {
		return c.download(ctx, url)
	}
	return nil, fmt.Errorf("%s returned neither image bytes nor a URL", c.model)
}

func (c *ImageClient) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching the image: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
