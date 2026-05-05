// go/integration_test.go
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/abilfida/openai-compatible-codebuddy/internal/types"
)

const baseURL = "http://localhost:3000"

func TestHealthEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	json.Unmarshal(body, &result)

	if result["status"] != "ok" {
		t.Errorf("expected ok, got %s", result["status"])
	}
}

func TestModelsEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL + "/v1/models")
	if err != nil {
		t.Fatalf("models request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result types.ModelListResponse
	json.Unmarshal(body, &result)

	if result.Object != "list" {
		t.Errorf("expected list, got %s", result.Object)
	}
}

func TestChatCompletionNonStreaming(t *testing.T) {
	reqBody := `{
		"model": "deepseek-v3.1",
		"messages": [{"role": "user", "content": "Say hello"}]
	}`

	resp, err := http.Post(baseURL+"/v1/chat/completions",
		"application/json",
		strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("chat request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Check cache header
	cacheHeader := resp.Header.Get("X-Cache")
	if cacheHeader != "MISS" && cacheHeader != "HIT" {
		t.Errorf("expected X-Cache header, got %s", cacheHeader)
	}

	body, _ := io.ReadAll(resp.Body)
	var result types.ChatCompletionResponse
	json.Unmarshal(body, &result)

	if result.Model != "deepseek-v3.1" {
		t.Errorf("expected deepseek-v3.1, got %s", result.Model)
	}
}