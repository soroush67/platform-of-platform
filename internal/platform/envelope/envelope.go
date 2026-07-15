// Package envelope implements the one at-rest encryption primitive this
// codebase needs for "a secret that has no further backend to defer
// to" (docs/architecture/11-module-secrets-state.md §1's own framing):
// a SecretMount's own bootstrap credential (e.g. a Vault AppRole
// secret_id) has to live somewhere, and Postgres is that somewhere, so
// it's encrypted at rest rather than stored plaintext. Master key from
// the environment (a real KMS-backed key in a real deployment, per the
// doc's own "master key from environment/KMS" framing - this codebase
// only ever reads it from an env var, the KMS integration itself is a
// further, out-of-scope addition, same posture already applied to
// MASTER_KEY/JWT_SIGNING_KEY elsewhere in this codebase).
//
// Per-record key derivation (not a wrapped-DEK envelope scheme): each
// record gets a random 16-byte salt at encryption time, and the actual
// AES key used for that one record is BLAKE2b-256(key=masterKey,
// data=salt) - a keyed hash used as a KDF, not a MAC. This means
// compromising one record's derived key (impossible without the master
// key anyway) never helps derive any other record's key, and rotating
// the master key just means re-deriving and re-encrypting every record
// once (a real, if not yet built, operational path - not silently
// assumed away).
package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/blake2b"
)

// KeySize is the required length of the master key (AES-256).
const KeySize = 32

// SaltSize - 16 bytes is standard practice for a KDF salt (more than
// enough collision resistance for this codebase's real record volumes,
// and blake2b.New256's own key-size limit is 64 bytes, well above this).
const SaltSize = 16

var ErrInvalidMasterKey = errors.New("envelope: master key must be exactly 32 bytes")

// Sealed is what gets stored - ciphertext, the GCM nonce, and the salt
// used to derive this one record's key. None of these three values is
// sensitive on its own (the whole point of envelope encryption): without
// the master key, the salt alone can't derive anything.
type Sealed struct {
	Ciphertext []byte
	Nonce      []byte
	Salt       []byte
}

// Seal encrypts plaintext under a key derived from masterKey and a
// fresh random salt - real, generated here, not derived from the
// record's own id (a UUID isn't secret, but using it as the sole KDF
// input would mean every re-encryption of the same record derives the
// identical key; a fresh random salt per Seal call avoids that for
// free).
func Seal(masterKey, plaintext []byte) (*Sealed, error) {
	if len(masterKey) != KeySize {
		return nil, ErrInvalidMasterKey
	}

	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("envelope: generating salt: %w", err)
	}

	recordKey, err := deriveKey(masterKey, salt)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(recordKey)
	if err != nil {
		return nil, fmt.Errorf("envelope: constructing AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope: constructing GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("envelope: generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return &Sealed{Ciphertext: ciphertext, Nonce: nonce, Salt: salt}, nil
}

// Open reverses Seal - re-derives the same per-record key from masterKey
// and the stored salt, then decrypts. A wrong masterKey (or a tampered
// ciphertext/nonce/salt) fails GCM's own authentication check, not a
// silent garbage decrypt.
func Open(masterKey []byte, sealed *Sealed) ([]byte, error) {
	if len(masterKey) != KeySize {
		return nil, ErrInvalidMasterKey
	}

	recordKey, err := deriveKey(masterKey, sealed.Salt)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(recordKey)
	if err != nil {
		return nil, fmt.Errorf("envelope: constructing AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope: constructing GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, sealed.Nonce, sealed.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("envelope: decryption failed (wrong master key, or tampered data): %w", err)
	}
	return plaintext, nil
}

func deriveKey(masterKey, salt []byte) ([]byte, error) {
	h, err := blake2b.New256(masterKey)
	if err != nil {
		return nil, fmt.Errorf("envelope: constructing blake2b KDF: %w", err)
	}
	h.Write(salt)
	return h.Sum(nil), nil
}
