package probe

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecryptInteraction(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	plaintext := []byte(`{"protocol":"http","remote-address":"192.0.2.1"}`)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	encryptedBody := make([]byte, len(plaintext))
	cipher.NewCTR(block, iv).XORKeyStream(encryptedBody, plaintext)
	ciphertext := append(append([]byte(nil), iv...), encryptedBody...)
	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &privateKey.PublicKey, key, nil)
	if err != nil {
		t.Fatal(err)
	}

	client := &Client{privateKey: privateKey}
	got, err := client.decrypt(base64.StdEncoding.EncodeToString(encryptedKey), base64.StdEncoding.EncodeToString(ciphertext))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("decrypt() = %q, want %q", got, plaintext)
	}
}

func TestNormalizeServerURL(t *testing.T) {
	parsed, err := normalizeServerURL("oast.pro")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.String() != "https://oast.pro" {
		t.Fatalf("normalizeServerURL() = %q", parsed)
	}
	if _, err := normalizeServerURL("http://oast.pro"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("http server error = %v", err)
	}
}

func TestAppendInteraction(t *testing.T) {
	interactions := appendInteraction(nil, []byte(`{"protocol":"dns","q-type":"A"}`))
	if len(interactions) != 1 || interactions[0].Protocol != "dns" || interactions[0].QType != "A" {
		t.Fatalf("unexpected interactions: %+v", interactions)
	}
	if got := appendInteraction(interactions, []byte(`not json`)); len(got) != 1 {
		t.Fatalf("invalid JSON changed interactions: %+v", got)
	}
}
