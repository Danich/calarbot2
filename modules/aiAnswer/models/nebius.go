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

// NebiusClient wraps the Nebius AI API for vision (image description) and image generation.
type NebiusClient struct {
	apiKey        string
	baseURL       string
	visionModel   string
	imageGenModel string
	httpClient    *http.Client
}

// NewNebiusClient creates a new NebiusClient.
func NewNebiusClient(apiKey, baseURL, visionModel, imageGenModel string) *NebiusClient {
	return &NebiusClient{
		apiKey:        apiKey,
		baseURL:       baseURL,
		visionModel:   visionModel,
		imageGenModel: imageGenModel,
		httpClient:    &http.Client{},
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

// GenerateImage creates an image from the prompt using the Nebius image-generation model
// and returns the URL of the first generated image.
func (c *NebiusClient) GenerateImage(ctx context.Context, prompt string) (string, error) {
	cl := c.newClient()
	res, err := cl.Images.Generate(ctx, openai.ImageGenerateParams{
		Prompt: prompt,
		Model:  openai.ImageModel(c.imageGenModel),
		N:      param.NewOpt[int64](1),
	})
	if err != nil {
		return "", err
	}
	if len(res.Data) == 0 {
		return "", fmt.Errorf("no image returned from Nebius")
	}
	if res.Data[0].URL == "" {
		return "", fmt.Errorf("empty URL in Nebius response")
	}
	return res.Data[0].URL, nil
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
