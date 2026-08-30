package models_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"calarbot2/modules/aiAnswer/models"
)

func TestImageClientGenerateImage(t *testing.T) {
	var gotPath, gotModel string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"url": "https://example.com/generated.jpg"}},
		})
	}))
	defer apiServer.Close()

	client := models.NewImageClient("test-key", apiServer.URL+"/", "some/image-model")
	url, err := client.GenerateImage(context.Background(), "a dog in a park")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if url != "https://example.com/generated.jpg" {
		t.Errorf("got %q, want %q", url, "https://example.com/generated.jpg")
	}
	if !strings.HasSuffix(gotPath, "/images/generations") {
		t.Errorf("запрос ушёл на %q, а не на images/generations", gotPath)
	}
	if gotModel != "some/image-model" {
		t.Errorf("модель в запросе = %q", gotModel)
	}
}

// Ненастроенный сервис — это не сбой провайдера, и путать их не надо: иначе
// пустой ключ приезжает как 401 откуда-то из глубины клиента.
func TestImageClientWithoutConfigurationSaysSo(t *testing.T) {
	for name, c := range map[string]*models.ImageClient{
		"без ключа":  models.NewImageClient("", "https://example.com/", "some/model"),
		"без модели": models.NewImageClient("k", "https://example.com/", ""),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := c.GenerateImage(context.Background(), "a dog")
			if err == nil || !strings.Contains(err.Error(), "not configured") {
				t.Fatalf("ошибка = %v, ожидалось «not configured»", err)
			}
		})
	}
}

func TestImageClientReportsAnEmptyResponse(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer apiServer.Close()

	client := models.NewImageClient("k", apiServer.URL+"/", "some/image-model")
	if _, err := client.GenerateImage(context.Background(), "a dog"); err == nil {
		t.Fatal("пустой ответ не превратился в ошибку")
	}
}
