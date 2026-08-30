package models_test

import (
	"context"
	"encoding/base64"
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
		// Именно b64_json: OpenRouter отдаёт картинку только так, поля url в
		// его ответе нет вовсе.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"b64_json": base64.StdEncoding.EncodeToString([]byte("PNGDATA"))},
			},
		})
	}))
	defer apiServer.Close()

	client := models.NewImageClient("test-key", apiServer.URL+"/", "some/image-model")
	img, err := client.GenerateImage(context.Background(), "a dog in a park")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(img) != "PNGDATA" {
		t.Errorf("got %q, want %q", img, "PNGDATA")
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

// Ветка для провайдеров, которые всё-таки отдают ссылку.
func TestImageClientFollowsAURLWhenThereIsOne(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer fileServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"url": fileServer.URL + "/img.png"}},
		})
	}))
	defer apiServer.Close()

	client := models.NewImageClient("k", apiServer.URL+"/", "some/image-model")
	img, err := client.GenerateImage(context.Background(), "a dog")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(img) != "PNGDATA" {
		t.Errorf("got %q", img)
	}
}

// Ответ без картинки вообще — это ошибка, а не пустой успех.
func TestImageClientRejectsAResponseWithNeither(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{}]}`))
	}))
	defer apiServer.Close()

	client := models.NewImageClient("k", apiServer.URL+"/", "some/image-model")
	if _, err := client.GenerateImage(context.Background(), "a dog"); err == nil {
		t.Fatal("ответ без байтов и без ссылки не превратился в ошибку")
	}
}
