package botModules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type ModuleClient struct {
	BaseURL string
}

// Register спрашивает модуль, кто он и какие у него настройки.
//
// Недоступный модуль получает Order 9999 — то же «в конец очереди», что было у
// Order(), — но теперь вместе с ошибкой: движку есть что записать в лог.
func (c *ModuleClient) Register() (Registration, error) {
	fallback := Registration{Order: 9999}

	resp, err := http.Get(c.BaseURL + "/register")
	if err != nil {
		return fallback, fmt.Errorf("register %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fallback, fmt.Errorf("register %s: status %d", c.BaseURL, resp.StatusCode)
	}

	var reg Registration
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return fallback, fmt.Errorf("decode registration from %s: %w", c.BaseURL, err)
	}
	return reg, nil
}

func (c *ModuleClient) IsCalled(msg *Payload) (bool, error) {
	url := c.BaseURL + "/is_called"
	body, _ := json.Marshal(msg)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Called bool `json:"called"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Called, nil
}

func (c *ModuleClient) Answer(msg *Payload) (RichAnswer, error) {
	url := c.BaseURL + "/answer"
	body, _ := json.Marshal(msg)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return RichAnswer{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Answer   string `json:"answer"`
		PhotoURL string `json:"photo_url,omitempty"`
		Photo    []byte `json:"photo,omitempty"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return RichAnswer{}, err
	}
	if result.Error != "" {
		return RichAnswer{Text: result.Answer, PhotoURL: result.PhotoURL, Photo: result.Photo}, fmt.Errorf("%s", result.Error)
	}
	return RichAnswer{Text: result.Answer, PhotoURL: result.PhotoURL, Photo: result.Photo}, nil
}
