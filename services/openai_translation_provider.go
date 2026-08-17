package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/osmansam/autotableGo/models"
)

type TranslationOutput struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type openAIResponse struct {
	Output []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

func decodeOpenAITranslations(body []byte) ([]TranslationOutput, error) {
	var response openAIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	for _, output := range response.Output {
		for _, content := range output.Content {
			if content.Type != "output_text" {
				continue
			}
			var result struct {
				Translations []TranslationOutput `json:"translations"`
			}
			if err := json.Unmarshal([]byte(content.Text), &result); err != nil {
				return nil, err
			}
			return result.Translations, nil
		}
	}
	return nil, fmt.Errorf("OpenAI response did not contain output_text")
}

func TranslateWithOpenAI(ctx context.Context, sourceLocale, targetLocale string, items []models.SourceString) ([]TranslationOutput, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("OPENAI_TRANSLATION_MODEL")
	if apiKey == "" || model == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY and OPENAI_TRANSLATION_MODEL are required")
	}
	input, _ := json.Marshal(items)
	payload := map[string]interface{}{
		"model": model,
		"input": fmt.Sprintf("Translate these UI labels from %s to %s. Preserve placeholders and return every key exactly once. Context is descriptive only. Items: %s", sourceLocale, targetLocale, input),
		"text": map[string]interface{}{"format": map[string]interface{}{
			"type": "json_schema", "name": "project_translations", "strict": true,
			"schema": map[string]interface{}{
				"type": "object", "additionalProperties": false, "required": []string{"translations"},
				"properties": map[string]interface{}{"translations": map[string]interface{}{
					"type": "array", "items": map[string]interface{}{
						"type": "object", "additionalProperties": false, "required": []string{"key", "text"},
						"properties": map[string]interface{}{"key": map[string]string{"type": "string"}, "text": map[string]string{"type": "string"}},
					},
				}},
			},
		}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 90 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenAI returned status %d: %s", response.StatusCode, string(responseBody))
	}
	return decodeOpenAITranslations(responseBody)
}
