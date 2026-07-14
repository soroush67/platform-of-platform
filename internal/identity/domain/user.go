// Package domain holds the Identity context's pure Go types.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

var ErrUserNotFound = errors.New("user not found")

// AuthSource is a closed set (docs/architecture/03-domain-model.md §3) -
// a Go type instead of a bare string so an invalid value is a
// compile-reachable mistake in this package's own tests, the same
// "closed set -> real type" reasoning Stage 5 §2 applied to
// runs.status as a Postgres enum.
type AuthSource string

const (
	AuthSourceLocal AuthSource = "local"
	AuthSourceOIDC  AuthSource = "oidc"
	AuthSourceSAML  AuthSource = "saml"
	AuthSourceLDAP  AuthSource = "ldap"
)

func (a AuthSource) valid() bool {
	switch a {
	case AuthSourceLocal, AuthSourceOIDC, AuthSourceSAML, AuthSourceLDAP:
		return true
	}
	return false
}

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// User is the Identity context's aggregate root - platform-global (can
// belong to multiple Organizations via OrganizationMembership, not
// itself org-scoped), per docs/architecture/03-domain-model.md §3. No
// RLS on the users table for the same reason - see migrations/0001_init.up.sql.
type User struct {
	ID          string
	Username    string
	Email       string
	AuthSource  AuthSource
	ExternalID  *string
	Status      string
	MFAEnrolled bool
	CreatedAt   time.Time
}

func NewUser(username, email string, authSource AuthSource) (*User, error) {
	if username == "" {
		return nil, &ValidationError{Message: "username is required"}
	}
	if !emailPattern.MatchString(email) {
		return nil, &ValidationError{Message: fmt.Sprintf("email %q is not valid", email)}
	}
	if !authSource.valid() {
		return nil, &ValidationError{Message: fmt.Sprintf("auth_source %q must be one of local, oidc, saml, ldap", authSource)}
	}

	return &User{
		ID:          uuid.NewString(),
		Username:    username,
		Email:       email,
		AuthSource:  authSource,
		Status:      "active",
		MFAEnrolled: false,
		CreatedAt:   time.Now().UTC(),
	}, nil
}
