package services

import "testing"

func TestDecodeOpenAITranslationsReadsStructuredOutputText(t *testing.T) {
	body := []byte(`{"output":[{"content":[{"type":"output_text","text":"{\"translations\":[{\"key\":\"page:1.name\",\"text\":\"Siparişler\"}]}"}]}]}`)
	items, err := decodeOpenAITranslations(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Key != "page:1.name" || items[0].Text != "Siparişler" {
		t.Fatalf("decoded translations = %#v", items)
	}
}
