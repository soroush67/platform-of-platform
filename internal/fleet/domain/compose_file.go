package domain

import (
	"time"

	"github.com/google/uuid"
)

// ComposeFile is the ported Python product's own "Profile" (renamed
// there before this port started) - a real docker-compose.yml's raw
// text plus its own attached Networks/Volumes/Variables. No GitLab
// source fields (source_type/gitlab_url/gitlab_ref/gitlab_path/
// gitlab_credential_id in the Python model) - GitLab ingestion is a
// deliberately deferred phase; every ComposeFile in this phase is the
// "upload" shape only.
//
// IsGlobal marks the Organization's own fallback ComposeFile for
// variable resolution - at most one per Organization, enforced by
// migrations/0019_fleet.up.sql's own partial unique index
// (compose_files_one_global_per_org), not a process-wide singleton the
// way the Python product's single-tenant seed was. An Organization with
// none yet simply has no global fallback - resolveComposeVariables
// skips that cascade step, not an error.
type ComposeFile struct {
	ID             string
	OrganizationID string
	Name           string
	IsGlobal       bool
	ComposeContent string
	CreatedBy      string
	CreatedAt      time.Time
}

func NewComposeFile(organizationID, name, composeContent, createdBy string, isGlobal bool) (*ComposeFile, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if createdBy == "" {
		return nil, &ValidationError{Message: "created_by is required"}
	}

	return &ComposeFile{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		Name:           name,
		IsGlobal:       isGlobal,
		ComposeContent: composeContent,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

func (c *ComposeFile) SetContent(content string) {
	c.ComposeContent = content
}
