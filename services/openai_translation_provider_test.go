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

func TestTranslateWithOpenAIProtectedTokensAreComparedExactly(t *testing.T) {
	for _, test := range []struct {
		name       string
		source     string
		translated string
		wantError  bool
	}{
		{name: "template preserved", source: "Hello {{name}}", translated: "Merhaba {{name}}"},
		{name: "template changed", source: "Hello {{name}}", translated: "Merhaba {{isim}}", wantError: true},
		{name: "url preserved", source: "Visit https://example.com/a", translated: "https://example.com/a adresini ziyaret edin"},
		{name: "identifier changed", source: "Use orderItem.value", translated: "order.value kullanın", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateProtectedTranslation(test.source, test.translated)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}
