// Package auth issues and verifies the short-lived JWT access token
// every credential type resolves to (docs/architecture/04-api-design.md
// §4) - cross-cutting, no domain knowledge, lives under
// internal/platform per docs/architecture/18-backend-structure.md §1.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid or expired token")

// AccessTokenTTL matches Stage 4 §4's "short-lived JWT access token" -
// short specifically because there's no revocation list in this walking
// skeleton yet; a stolen token self-invalidates quickly rather than
// staying valid indefinitely.
const AccessTokenTTL = 15 * time.Minute

type claims struct {
	jwt.RegisteredClaims
}

// IssueAccessToken mints a token whose only claim is the subject (the
// User's id) - it deliberately does NOT embed an organization_id
// (unlike Stage 4 §4's full Principal{subject, organization_id,
// permissions}) because a User can belong to multiple orgs
// (docs/architecture/03-domain-model.md §3) and this walking skeleton
// resolves which org per-request from the URL, verified against
// OrganizationMembership - baking one org into the token itself would
// be wrong the moment a user acts on a second org.
func IssueAccessToken(secret []byte, userID string) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
		},
	})
	return token.SignedString(secret)
}

// ParseAccessToken verifies signature and expiry, returns the user id.
func ParseAccessToken(secret []byte, tokenString string) (string, error) {
	parsed, err := jwt.ParseWithClaims(tokenString, &claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method)
		}
		return secret, nil
	})
	if err != nil || !parsed.Valid {
		return "", ErrInvalidToken
	}

	c, ok := parsed.Claims.(*claims)
	if !ok || c.Subject == "" {
		return "", ErrInvalidToken
	}

	return c.Subject, nil
}
