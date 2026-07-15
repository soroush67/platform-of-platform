package envelope_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"platform-of-platform/internal/platform/envelope"
)

func mustMasterKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, envelope.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return key
}

func TestSealAndOpen_RoundTrips(t *testing.T) {
	masterKey := mustMasterKey(t)
	plaintext := []byte("a real Vault AppRole secret_id")

	sealed, err := envelope.Seal(masterKey, plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed.Ciphertext, plaintext) {
		t.Fatal("expected the ciphertext to not contain the plaintext verbatim")
	}

	got, err := envelope.Open(masterKey, sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("expected the round-tripped plaintext to match, got %q, want %q", got, plaintext)
	}
}

func TestSeal_TwoCallsProduceDifferentSaltsAndCiphertexts(t *testing.T) {
	masterKey := mustMasterKey(t)
	plaintext := []byte("same plaintext both times")

	first, err := envelope.Seal(masterKey, plaintext)
	if err != nil {
		t.Fatalf("Seal (first): %v", err)
	}
	second, err := envelope.Seal(masterKey, plaintext)
	if err != nil {
		t.Fatalf("Seal (second): %v", err)
	}

	if bytes.Equal(first.Salt, second.Salt) {
		t.Error("expected two Seal calls to use different random salts")
	}
	if bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Error("expected two Seal calls on the same plaintext to produce different ciphertexts")
	}
}

func TestOpen_WrongMasterKeyFails(t *testing.T) {
	masterKey := mustMasterKey(t)
	wrongKey := mustMasterKey(t)
	sealed, err := envelope.Seal(masterKey, []byte("secret"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if _, err := envelope.Open(wrongKey, sealed); err == nil {
		t.Fatal("expected Open with the wrong master key to fail, not silently return garbage")
	}
}

func TestOpen_TamperedCiphertextFails(t *testing.T) {
	masterKey := mustMasterKey(t)
	sealed, err := envelope.Seal(masterKey, []byte("secret"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	sealed.Ciphertext[0] ^= 0xFF

	if _, err := envelope.Open(masterKey, sealed); err == nil {
		t.Fatal("expected Open on tampered ciphertext to fail GCM authentication")
	}
}

func TestSeal_RejectsWrongSizedMasterKey(t *testing.T) {
	_, err := envelope.Seal([]byte("too-short"), []byte("secret"))
	if err != envelope.ErrInvalidMasterKey {
		t.Fatalf("expected ErrInvalidMasterKey, got: %v", err)
	}
}
