package handlers

import (
	"context"
	"fmt"

	"calarbot2/botModules"
)

type ImageGenClient interface {
	GenerateImage(ctx context.Context, prompt string) ([]byte, error)
}

type ImageGenHandler struct {
	client ImageGenClient
}

func NewImageGenHandler(client ImageGenClient) *ImageGenHandler {
	return &ImageGenHandler{client: client}
}

// Generate returns a RichAnswer carrying the generated image.
func (h *ImageGenHandler) Generate(ctx context.Context, prompt string) (botModules.RichAnswer, error) {
	if prompt == "" {
		return botModules.RichAnswer{}, fmt.Errorf("empty prompt")
	}
	img, err := h.client.GenerateImage(ctx, prompt)
	if err != nil {
		return botModules.RichAnswer{}, err
	}
	return botModules.RichAnswer{Photo: img}, nil
}
