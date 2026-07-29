package auth

import (
	"encoding/hex"
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() returned error: %v", err)
	}
	raw, err := hex.DecodeString(key)
	if err != nil {
		t.Fatalf("GenerateAPIKey() = %q, not valid hex: %v", key, err)
	}
	if len(raw) != 32 {
		t.Errorf("GenerateAPIKey() decodes to %d bytes, want 32", len(raw))
	}

	second, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() second call returned error: %v", err)
	}
	if key == second {
		t.Errorf("GenerateAPIKey() returned the same key twice: %q", key)
	}
}

func TestHashAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "empty string",
			key:  "",
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name: "known vector abc",
			key:  "abc",
			want: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
		{
			name: "hex-looking key",
			key:  "68656c6c6f",
			want: "4ef79bf561cdeacd465e135a3b9c8b51a42ded0605f15ab8e501162d2693bd00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HashAPIKey(tt.key)
			if got != tt.want {
				t.Errorf("HashAPIKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
			if got == tt.key {
				t.Errorf("HashAPIKey(%q) returned its input", tt.key)
			}
		})
	}
}
