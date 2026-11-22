package translate

import (
	"strings"
	"testing"
)

func TestSplitText(t *testing.T) {
	text := strings.Repeat("Это предложение. ", 100)
	chunks := splitText(text, 120)
	if len(chunks) < 2 {
		t.Fatal("expected multiple chunks")
	}
	for _, c := range chunks {
		if len([]rune(c)) > 120 {
			t.Fatalf("chunk too long: %d", len([]rune(c)))
		}
	}
	joined := strings.Join(chunks, " ")
	if !strings.Contains(joined, "предложение") {
		t.Fatal("content lost during split")
	}
}

func TestCollectStrings(t *testing.T) {
	var b strings.Builder
	collectStrings([]any{"Привет", []any{" мир", "!"}}, &b)
	if b.String() != "Привет мир!" {
		t.Fatalf("unexpected %q", b.String())
	}
}
