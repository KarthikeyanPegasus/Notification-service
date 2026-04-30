package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const vendorConfigEnvelopeVersion = "aes256gcm_v1"

type vendorConfigEnvelope struct {
	Enc        string `json:"_enc"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type VendorConfigCrypto struct {
	aead cipher.AEAD
}

// NewVendorConfigCrypto creates an encrypt/decrypt helper for vendor configs.
// key can be base64-encoded 32 bytes (recommended) or a raw 32-byte string.
func NewVendorConfigCrypto(key string) (*VendorConfigCrypto, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("vendor config encryption key is empty")
	}

	var keyBytes []byte
	if decoded, err := base64.StdEncoding.DecodeString(key); err == nil {
		keyBytes = decoded
	} else {
		// Fall back to raw string. This is less safe operationally, but avoids breaking dev setups.
		keyBytes = []byte(key)
	}

	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("vendor config encryption key must be 32 bytes (got %d)", len(keyBytes))
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating gcm: %w", err)
	}
	return &VendorConfigCrypto{aead: aead}, nil
}

func (c *VendorConfigCrypto) IsEncrypted(b json.RawMessage) bool {
	var env vendorConfigEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return false
	}
	return env.Enc == vendorConfigEnvelopeVersion && env.Nonce != "" && env.Ciphertext != ""
}

// EncryptJSON encrypts plaintext JSON and returns an encrypted JSON envelope.
func (c *VendorConfigCrypto) EncryptJSON(plaintext json.RawMessage) (json.RawMessage, error) {
	if !json.Valid(plaintext) {
		return nil, fmt.Errorf("plaintext is not valid json")
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("reading nonce: %w", err)
	}

	ciphertext := c.aead.Seal(nil, nonce, plaintext, nil)

	env := vendorConfigEnvelope{
		Enc:        vendorConfigEnvelopeVersion,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	out, err := json.Marshal(&env)
	if err != nil {
		return nil, fmt.Errorf("marshalling envelope: %w", err)
	}
	return json.RawMessage(out), nil
}

// DecryptJSON decrypts an encrypted JSON envelope and returns plaintext JSON.
// If the input is not an encrypted envelope, it returns (input, false, nil).
func (c *VendorConfigCrypto) DecryptJSON(b json.RawMessage) (json.RawMessage, bool, error) {
	var env vendorConfigEnvelope
	if err := json.Unmarshal(b, &env); err != nil || env.Enc == "" {
		return b, false, nil
	}

	if env.Enc != vendorConfigEnvelopeVersion {
		return nil, true, fmt.Errorf("unsupported vendor config envelope version: %s", env.Enc)
	}

	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, true, fmt.Errorf("decoding nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, true, fmt.Errorf("decoding ciphertext: %w", err)
	}

	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, true, fmt.Errorf("decrypting vendor config: %w", err)
	}
	if !json.Valid(plaintext) {
		return nil, true, fmt.Errorf("decrypted vendor config is not valid json")
	}
	return json.RawMessage(plaintext), true, nil
}

