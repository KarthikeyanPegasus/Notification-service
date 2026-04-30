package security

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestVendorConfigCrypto_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := NewVendorConfigCrypto(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewVendorConfigCrypto: %v", err)
	}

	plain := json.RawMessage(`{"api_key":"secret","nested":{"x":1}}`)
	enc, err := c.EncryptJSON(plain)
	if err != nil {
		t.Fatalf("EncryptJSON: %v", err)
	}
	if !c.IsEncrypted(enc) {
		t.Fatalf("expected encrypted envelope")
	}

	out, wasEncrypted, err := c.DecryptJSON(enc)
	if err != nil {
		t.Fatalf("DecryptJSON: %v", err)
	}
	if !wasEncrypted {
		t.Fatalf("expected wasEncrypted=true")
	}
	if string(out) != string(plain) {
		t.Fatalf("mismatch: got=%s want=%s", out, plain)
	}
}

func TestVendorConfigCrypto_DecryptPassthrough(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(0xAA)
	}
	c, err := NewVendorConfigCrypto(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewVendorConfigCrypto: %v", err)
	}

	plain := json.RawMessage(`{"hello":"world"}`)
	out, wasEncrypted, err := c.DecryptJSON(plain)
	if err != nil {
		t.Fatalf("DecryptJSON: %v", err)
	}
	if wasEncrypted {
		t.Fatalf("expected wasEncrypted=false")
	}
	if string(out) != string(plain) {
		t.Fatalf("mismatch")
	}
}

