package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestHMACSHA256MatchesReferenceVector(t *testing.T) {
	got := HMACSHA256("The quick brown fox jumps over the lazy dog", "key")
	want, _ := hex.DecodeString("f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8")
	if !hmac.Equal(got, want) {
		t.Fatalf("unexpected digest: %s", hex.EncodeToString(got))
	}
}

func TestNormalizeCDKUppersAndStrips(t *testing.T) {
	cases := map[string]string{
		" cf-12 ab CD ": "CF12ABCD",
		"captcha":       "CAPTCHA",
		"----":          "",
		"":              "",
	}
	for input, want := range cases {
		if got := NormalizeCDK(input); got != want {
			t.Fatalf("NormalizeCDK(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGenerateOpaqueTokenShape(t *testing.T) {
	token, err := GenerateOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	// 32 random bytes -> 43 base64url characters without padding.
	if len(token) != 43 || strings.ContainsAny(token, "+/=") {
		t.Fatalf("unexpected token shape: %q", token)
	}
}

func TestNewAPIKeySecretFormat(t *testing.T) {
	secret, err := NewAPIKeySecret()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "cf_live_") {
		t.Fatalf("secret missing prefix: %q", secret)
	}
	if len(secret) != len("cf_live_")+43 {
		t.Fatalf("unexpected secret length: %d", len(secret))
	}
}

func TestNewRequestIDFormat(t *testing.T) {
	id := NewRequestID()
	if !strings.HasPrefix(id, "req_") || len(id) != len("req_")+32 {
		t.Fatalf("unexpected request id: %q", id)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(id, "req_")); err != nil {
		t.Fatalf("request id suffix not hex: %q", id)
	}
	if NewRequestID() == id {
		t.Fatal("request ids must be unique")
	}
}

var _ = sha256.Size
