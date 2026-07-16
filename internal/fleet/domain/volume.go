package domain

import (
	"time"

	"github.com/google/uuid"
)

// Volume is an admin-managed catalog entry - a host bind-mount source,
// referenced by ComposeFiles via VolumeAttachment (which also carries
// the per-attachment container_path, since that's specific to how each
// ComposeFile mounts it, not to the Volume itself).
type Volume struct {
	ID             string
	OrganizationID string
	Name           string
	HostPath       string
	CreatedBy      string
	CreatedAt      time.Time
}

func NewVolume(organizationID, name, hostPath, createdBy string) (*Volume, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if hostPath == "" {
		return nil, &ValidationError{Message: "host_path is required"}
	}
	if createdBy == "" {
		return nil, &ValidationError{Message: "created_by is required"}
	}

	return &Volume{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		Name:           name,
		HostPath:       hostPath,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now().UTC(),
	}, nil
}
