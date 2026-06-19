package models

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"calarbot2/modules/aiAnswer/router"
)

const (
	defaultTopModelsURL = "https://shir-man.com/api/free-llm/top-models"
	FallbackModel       = "openrouter/free"
	openrouterBaseURL   = "https://openrouter.ai/api/v1/"
	refreshInterval     = 24 * time.Hour
)

type MetaStore interface {
	GetMeta(key string) (string, bool, error)
	SetMeta(key, value string) error
}

type ModelSelector struct {
	mu           sync.RWMutex
	model        string
	store        MetaStore
	httpClient   *http.Client
	topModelsURL string
}

func NewModelSelector(store MetaStore, topModelsURL string) *ModelSelector {
	if topModelsURL == "" {
		topModelsURL = defaultTopModelsURL
	}
	ms := &ModelSelector{
		store:        store,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		topModelsURL: topModelsURL,
	}
	if cached, ok, err := store.GetMeta("top_model"); err == nil && ok && cached != "" {
		ms.model = cached
	}
	return ms
}

func (ms *ModelSelector) Get() string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.model == "" {
		return FallbackModel
	}
	return ms.model
}

func (ms *ModelSelector) Refresh() {
	resp, err := ms.httpClient.Get(ms.topModelsURL)
	if err != nil {
		log.Printf("shir-man.com fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Models) == 0 {
		log.Printf("shir-man.com parse error: %v", err)
		return
	}

	model := result.Models[0].ID
	ms.mu.Lock()
	ms.model = model
	ms.mu.Unlock()

	_ = ms.store.SetMeta("top_model", model)
	_ = ms.store.SetMeta("top_model_updated_at", time.Now().Format(time.RFC3339))
	log.Printf("top model updated: %s", model)
}

func (ms *ModelSelector) StartRefresh(ctx context.Context) {
	ms.Refresh()
	go func() {
		t := time.NewTicker(refreshInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				ms.Refresh()
			case <-ctx.Done():
				return
			}
		}
	}()
}

const classifySystemPrompt = `Classify the user message into exactly one word (no punctuation, no explanation):
translate — user wants text translated
imagegen — user wants an image drawn or generated
vision — user wants an image described or analyzed
question — user has a factual question expecting an answer
chat — casual conversation or anything else`

type OpenRouterClient struct {
	apiKey  string
	sel     *ModelSelector
	baseURL string
}

func NewOpenRouterClient(apiKey string, sel *ModelSelector, baseURL string) *OpenRouterClient {
	if baseURL == "" {
		baseURL = openrouterBaseURL
	}
	return &OpenRouterClient{apiKey: apiKey, sel: sel, baseURL: baseURL}
}

func (c *OpenRouterClient) newClient() openai.Client {
	return openai.NewClient(
		option.WithAPIKey(c.apiKey),
		option.WithBaseURL(c.baseURL),
	)
}

func (c *OpenRouterClient) Complete(ctx context.Context, system, user string) (string, error) {
	cl := c.newClient()
	res, err := cl.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.sel.Get(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(system),
			openai.UserMessage(user),
		},
	})
	if err != nil {
		return "", err
	}
	return res.Choices[0].Message.Content, nil
}

func (c *OpenRouterClient) Classify(ctx context.Context, text string) (router.Route, error) {
	result, err := c.Complete(ctx, classifySystemPrompt, text)
	if err != nil {
		return router.RouteChat, err
	}
	switch strings.TrimSpace(strings.ToLower(result)) {
	case "translate":
		return router.RouteTranslate, nil
	case "imagegen":
		return router.RouteImageGen, nil
	case "vision":
		return router.RouteVision, nil
	case "question":
		return router.RouteQuestion, nil
	default:
		return router.RouteChat, nil
	}
}
