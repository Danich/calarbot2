package router_test

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/router"
)

type mockClassifier struct{ result router.Route }

func (m *mockClassifier) Classify(_ context.Context, _ string) (router.Route, error) {
	return m.result, nil
}

func txtMsg(text string) *tgbotapi.Message {
	return &tgbotapi.Message{Text: text}
}

func photoMsg(text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Text:  text,
		Photo: []tgbotapi.PhotoSize{{FileID: "abc"}},
	}
}

func TestRouterKeywordImageGen(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	for _, text := range []string{"нарисуй кота", "draw a cat", "сгенерируй картинку"} {
		route, err := r.Route(context.Background(), txtMsg(text))
		if err != nil || route != router.RouteImageGen {
			t.Errorf("text=%q: got %q err=%v, want imagegen", text, route, err)
		}
	}
}

func TestRouterKeywordTranslate(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	for _, text := range []string{"переведи это", "translate this", "перевести текст"} {
		route, _ := r.Route(context.Background(), txtMsg(text))
		if route != router.RouteTranslate {
			t.Errorf("text=%q: got %q, want translate", text, route)
		}
	}
}

func TestRouterKeywordVision(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	for _, text := range []string{"что на картинке", "распознай это", "опиши фото"} {
		route, _ := r.Route(context.Background(), txtMsg(text))
		if route != router.RouteVision {
			t.Errorf("text=%q: got %q, want vision", text, route)
		}
	}
}

func TestRouterPhotoWithoutText(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	route, _ := r.Route(context.Background(), photoMsg(""))
	if route != router.RouteVision {
		t.Errorf("photo with no text: got %q, want vision", route)
	}
}

func TestRouterPhotoWithImageGenKeyword(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	route, _ := r.Route(context.Background(), photoMsg("нарисуй похожее"))
	if route != router.RouteImageGen {
		t.Errorf("photo with imagegen keyword: got %q, want imagegen", route)
	}
}

func TestRouterFallsBackToClassifier(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteQuestion})
	route, _ := r.Route(context.Background(), txtMsg("what is the capital of France?"))
	if route != router.RouteQuestion {
		t.Errorf("got %q, want question", route)
	}
}

func TestRouterEmptyTextReturnsChat(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteQuestion})
	route, _ := r.Route(context.Background(), txtMsg(""))
	if route != router.RouteChat {
		t.Errorf("got %q, want chat", route)
	}
}
