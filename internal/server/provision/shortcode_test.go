package provision

import (
	"testing"
)

func TestGenerateShortCode_Length(t *testing.T) {
	code, err := GenerateShortCode()
	if err != nil {
		t.Fatalf("GenerateShortCode() error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("expected length 6, got %d: %q", len(code), code)
	}
}

func TestGenerateShortCode_ValidChars(t *testing.T) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	for i := 0; i < 100; i++ {
		code, err := GenerateShortCode()
		if err != nil {
			t.Fatalf("iteration %d: GenerateShortCode() error: %v", i, err)
		}
		for _, ch := range code {
			found := false
			for _, valid := range alphabet {
				if ch == valid {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("invalid character %q in code %q", string(ch), code)
			}
		}
	}
}

func TestGenerateShortCode_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code, err := GenerateShortCode()
		if err != nil {
			t.Fatalf("iteration %d: GenerateShortCode() error: %v", i, err)
		}
		if seen[code] {
			t.Errorf("duplicate code after %d iterations: %q", i, code)
		}
		seen[code] = true
	}
}

func TestValidateShortCode(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		valid bool
	}{
		{"valid code", "A3X9K2", true},
		{"lowercase", "a3x9k2", false},
		{"too short", "A3X9K", false},
		{"too long", "A3X9K2B", false},
		{"empty", "", false},
		{"contains O", "A3XOK2", false},
		{"contains 0", "A3X0K2", false},
		{"contains I", "A3XIK2", false},
		{"contains 1", "A3X1K2", false},
		{"contains L", "A3XLK2", false},
		{"all valid chars", "ABCDEF", true},
		{"all valid digits", "234567", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateShortCode(tt.code); got != tt.valid {
				t.Errorf("ValidateShortCode(%q) = %v, want %v", tt.code, got, tt.valid)
			}
		})
	}
}
