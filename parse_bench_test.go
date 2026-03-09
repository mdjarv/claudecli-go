package claudecli

import (
	"bytes"
	"os"
	"testing"
)

func BenchmarkParseBasicStream(b *testing.B) {
	data, err := os.ReadFile("testdata/basic.jsonl")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		ch := make(chan Event, 64)
		go func() {
			ParseEvents(bytes.NewReader(data), ch)
			close(ch)
		}()
		for range ch {
		}
	}
}

func BenchmarkParseToolUseStream(b *testing.B) {
	data, err := os.ReadFile("testdata/tool_use.jsonl")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		ch := make(chan Event, 64)
		go func() {
			ParseEvents(bytes.NewReader(data), ch)
			close(ch)
		}()
		for range ch {
		}
	}
}
