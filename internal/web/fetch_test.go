package web_test

import (
	"fmt"
	"github.com/sigpanic/goink/internal/web"
	"testing"
)

func TestFetch(t *testing.T) {
	result, err := web.Fetch("https://api-docs.deepseek.com/")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("URL:   %s\nTitle: %s\nText length: %d\n\n--- text ---\n%s\n",
		result.URL, result.Title, len([]rune(result.Text)), result.Text)
}
