package domain

import (
	"time"

	"github.com/google/uuid"
)

// Network is an admin-managed catalog entry - referenced (never
// duplicated) by ComposeFiles via NetworkAttachment. External mirrors
// real docker-compose `networks: <name>: {external: true}` semantics -
// a network this stack expects to already exist on the target host,
// not one it should create.
type Network struct {
	ID             string
	OrganizationID string
	Name           string
	External       bool
	CreatedBy      string
	CreatedAt      time.Time
}

func NewNetwork(organizationID, name, createdBy string, external bool) (*Network, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if createdBy == "" {
		return nil, &ValidationError{Message: "created_by is required"}
	}

	return &Network{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		Name:           name,
		External:       external,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now().UTC(),
	}, nil
}
