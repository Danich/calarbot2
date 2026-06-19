package handlers

import (
	"context"
	"fmt"

	"calarbot2/botModules"
)

type ImageGenClient interface {
	GenerateImage(ctx context.Context, prompt string) (string, error)
}

type ImageGenHandler struct {
	client ImageGenClient
}

func NewImageGenHandler(client ImageGenClient) *ImageGenHandler {
	return &ImageGenHandler{client: client}
}

// Generate returns a RichAnswer with the URL of the generated image.
func (h *ImageGenHandler) Generate(ctx context.Context, prompt string) (botModules.RichAnswer, error) {
	if prompt == "" {
		return botModules.RichAnswer{}, fmt.Errorf("empty prompt")
	}
	url, err := h.client.GenerateImage(ctx, prompt)
	if err != nil {
		return botModules.RichAnswer{}, err
	}
	return botModules.RichAnswer{PhotoURL: url, Text: ""}, nil
}
