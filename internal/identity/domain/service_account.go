package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrServiceAccountNotFound = errors.New("service account not found")

// ServiceAccount is org-scoped (unlike User, which is platform-global) -
// docs/architecture/03-domain-model.md §3: "distinct from User
// specifically because service accounts have no password/MFA/SSO
// concerns, only APIKey-based auth, and should never appear in a 'list
// human users' view."
type ServiceAccount struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
	CreatedAt      time.Time
}

func NewServiceAccount(organizationID, name, description string) (*ServiceAccount, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	return &ServiceAccount{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		Name:           name,
		Description:    description,
		CreatedAt:      time.Now().UTC(),
	}, nil
}
