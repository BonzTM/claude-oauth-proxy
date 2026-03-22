package openai

import (
	"encoding/json"
	"testing"
)

func TestMessageContentUnmarshalStringAndArray(t *testing.T) {
	var stringContent MessageContent
	if err := json.Unmarshal([]byte(`"hello"`), &stringContent); err != nil {
		t.Fatalf("unmarshal string: %v", err)
	}
	if stringContent.Text != "hello" {
		t.Fatalf("unexpected string content: %+v", stringContent)
	}
	var arrayContent MessageContent
	if err := json.Unmarshal([]byte(`[{"type":"text","text":"hello "},{"type":"text","text":"world"}]`), &arrayContent); err != nil {
		t.Fatalf("unmarshal array: %v", err)
	}
	if arrayContent.Text != "hello world" {
		t.Fatalf("unexpected array content: %+v", arrayContent)
	}
}

func TestMessageContentRejectsUnsupportedPartsAndMarshalsAsString(t *testing.T) {
	var content MessageContent
	if err := json.Unmarshal([]byte(`[{"type":"image_url"}]`), &content); err == nil {
		t.Fatal("expected unsupported content type error")
	}
	if err := json.Unmarshal([]byte(`null`), &content); err != nil || content.Text != "" {
		t.Fatalf("unexpected null content result: %+v err=%v", content, err)
	}
	if err := json.Unmarshal([]byte(`{`), &content); err == nil {
		t.Fatal("expected invalid json error")
	}
	encoded, err := json.Marshal(MessageContent{Text: "hello"})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	if string(encoded) != `"hello"` {
		t.Fatalf("unexpected encoded content: %s", string(encoded))
	}
}
