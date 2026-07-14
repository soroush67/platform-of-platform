package domain

import (
	"time"

	"github.com/google/uuid"
)

// OrganizationMembership is an entity, not a full aggregate - it's "this
// user is part of this org," a prerequisite RBAC bindings reference, not
// itself an authorization grant (docs/architecture/03-domain-model.md §2).
type OrganizationMembership struct {
	ID             string
	OrganizationID string
	UserID         string
	JoinedAt       time.Time
}

func NewOrganizationMembership(organizationID, userID string) *OrganizationMembership {
	return &OrganizationMembership{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		UserID:         userID,
		JoinedAt:       time.Now().UTC(),
	}
}
