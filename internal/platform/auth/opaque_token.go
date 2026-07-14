// opaque_token.go: the generation/hashing half shared by refresh tokens
// and password reset tokens (internal/identity) - both need the exact
// same "cryptographically random plaintext, only a hash ever touches
// the database" shape, so it lives here as a cross-cutting primitive
// rather than being duplicated per use case.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// GenerateOpaqueToken returns a 256-bit random token, base64url-encoded
// for use as plaintext (returned to the client exactly once) - crypto/rand,
// not math/rand: this is a bearer credential, not a display id.
func GenerateOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashOpaqueToken is what actually gets stored/compared - SHA-256, not
// bcrypt: unlike a user-chosen password, this token already has 256 bits
// of real entropy, so a slow, salted KDF defends against nothing extra
// here and would only add cost to every lookup.
func HashOpaqueToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
