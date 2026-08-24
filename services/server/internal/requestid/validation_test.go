package requestid

import (
	"strings"
	"testing"
)

func TestValid(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "uuid", value: "e6cf5fb0-c0e4-42d9-b6a8-74eb8533a563", want: true},
		{name: "unicode", value: "准备-请求-1", want: true},
		{name: "empty"},
		{name: "nul", value: "ready\x00request"},
		{name: "newline", value: "ready\nrequest"},
		{name: "too long", value: strings.Repeat("x", MaxLength+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Valid(test.value); got != test.want {
				t.Fatalf("Valid(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}
