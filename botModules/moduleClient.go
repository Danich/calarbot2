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

func (c *ModuleClient) Order() int {
	url := c.BaseURL + "/order"
	resp, err := http.Get(url)
	if err != nil {
		return 9999
	}
	defer resp.Body.Close()

	var result struct {
		Order int `json:"order"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Println("Error decoding order response:", err)
		return 9999
	}
	return result.Order

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

func (c *ModuleClient) Answer(msg *Payload) (string, error) {
	url := c.BaseURL + "/answer"
	body, _ := json.Marshal(msg)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Answer string `json:"answer"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return result.Answer, fmt.Errorf(result.Error)
	}
	return result.Answer, nil
}
