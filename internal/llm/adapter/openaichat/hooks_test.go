package openaichat

import "testing"

func TestMimoBuildHeaders(t *testing.T) {
	input := map[string]string{"Authorization": "Bearer sk-mimo-123"}
	result := mimoBuildHeaders(nil, input)
	if apiKey, ok := result["api-key"]; !ok || apiKey != "sk-mimo-123" {
		t.Errorf("api-key: got %q", result["api-key"])
	}
	if _, exists := result["Authorization"]; exists {
		t.Error("Authorization should be removed")
	}
	// Go map 是引用类型，原 map 也会被修改
	if _, exists := input["api-key"]; !exists {
		t.Error("api-key should exist in original map (go map is by reference)")
	}
}
