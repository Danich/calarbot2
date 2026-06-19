package router

import (
	"context"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Route string

const (
	RouteChat      Route = "chat"
	RouteQuestion  Route = "question"
	RouteTranslate Route = "translate"
	RouteVision    Route = "vision"
	RouteImageGen  Route = "imagegen"
)

type Classifier interface {
	Classify(ctx context.Context, text string) (Route, error)
}

type Router struct {
	classifier Classifier
}

func New(classifier Classifier) *Router {
	return &Router{classifier: classifier}
}

var (
	imagegenKeywords  = []string{"нарисуй", "draw ", "сгенерируй", "generate image"}
	translateKeywords = []string{"переведи", "translate ", "перевести", "переведите"}
	visionKeywords    = []string{"что на картинке", "распознай", "опиши фото", "опиши картинку"}
)

func messageText(msg *tgbotapi.Message) string {
	if msg.Text != "" {
		return msg.Text
	}
	return msg.Caption
}

func (r *Router) Route(ctx context.Context, msg *tgbotapi.Message) (Route, error) {
	text := strings.ToLower(messageText(msg))

	if containsAny(text, imagegenKeywords) {
		return RouteImageGen, nil
	}
	if containsAny(text, translateKeywords) {
		return RouteTranslate, nil
	}
	if containsAny(text, visionKeywords) {
		return RouteVision, nil
	}
	if msg.Photo != nil {
		return RouteVision, nil
	}
	if msg.Text == "" {
		return RouteChat, nil
	}
	return r.classifier.Classify(ctx, msg.Text)
}

func containsAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
