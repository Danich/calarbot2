package models_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"calarbot2/modules/aiAnswer/models"
)

func TestNebiusClientDescribeImage(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake image bytes"))
	}))
	defer imageServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "a cat sitting on a mat"}},
			},
		})
	}))
	defer apiServer.Close()

	client := models.NewNebiusClient("test-key", apiServer.URL+"/", "vision-model", "imagegen-model")
	desc, err := client.DescribeImage(context.Background(), imageServer.URL+"/image.jpg", "describe it")
	if err != nil {
		t.Fatalf("DescribeImage: %v", err)
	}
	if desc != "a cat sitting on a mat" {
		t.Errorf("got %q, want %q", desc, "a cat sitting on a mat")
	}
}

func TestNebiusClientGenerateImage(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"url": "https://example.com/generated.jpg"}},
		})
	}))
	defer apiServer.Close()

	client := models.NewNebiusClient("test-key", apiServer.URL+"/", "vision-model", "imagegen-model")
	url, err := client.GenerateImage(context.Background(), "a dog in a park")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if url != "https://example.com/generated.jpg" {
		t.Errorf("got %q, want %q", url, "https://example.com/generated.jpg")
	}
}
