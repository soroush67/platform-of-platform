package application

import (
	"context"
	"fmt"
	"sort"

	"platform-of-platform/internal/fleet/domain"
)

// CreateSecretVariableInput is deliberately a separate, narrower shape
// from CreateVariableInput - this service always creates a var_type=
// secret Variable and writes the operator-supplied real value into
// Vault itself (no pre-existing path required), rather than referencing
// a secret that must already exist at an exact path the operator
// already knows (CreateVariableService's own, unchanged, "secret_ref"
// flow).
type CreateSecretVariableInput struct {
	OrganizationID   string
	RequestingUserID string
	ComposeFileID    string
	Key              string
	MountID          string
	Value            string
}

// CreateSecretVariableService writes the same value into Vault once per
// Project this ComposeFile is currently linked to (a ComposeFile<->
// Project link is many-to-many - compose_file_projects) under
// secret/data/fleet/compose-files/{project_id}/{compose_file_id}/{key},
// mirroring MachinesPage's own existing secret/data/fleet/machines/
// {slug} convention. All copies hold the identical value - this app
// itself only ever reads back through the first linked project's path
// (sorted by ID for a deterministic choice), the rest exist purely so
// an operator browsing Vault directly under a given Project's own
// prefix finds the secret there too.
type CreateSecretVariableService struct {
	repo         VariableRepository
	attachments  AttachmentRepository
	membership   MembershipChecker
	permChecker  PermissionChecker
	mountCheck   SecretMountChecker
	secretWriter SecretWriter
}

func NewCreateSecretVariableService(repo VariableRepository, attachments AttachmentRepository, membership MembershipChecker, permChecker PermissionChecker, mountCheck SecretMountChecker, secretWriter SecretWriter) *CreateSecretVariableService {
	return &CreateSecretVariableService{
		repo: repo, attachments: attachments, membership: membership,
		permChecker: permChecker, mountCheck: mountCheck, secretWriter: secretWriter,
	}
}

func (s *CreateSecretVariableService) Execute(ctx context.Context, in CreateSecretVariableInput) (*domain.Variable, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionComposeFileManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	if in.Key == "" || in.MountID == "" || in.Value == "" {
		return nil, &domain.ValidationError{Message: "key, mount_id and value are all required"}
	}

	mountExists, err := s.mountCheck.SecretMountExists(ctx, in.OrganizationID, in.MountID)
	if err != nil {
		return nil, err
	}
	if !mountExists {
		return nil, &domain.ValidationError{Message: "mount_id does not reference a real secret mount in this organization"}
	}

	projects, err := s.attachments.ListProjectsForComposeFile(ctx, in.OrganizationID, in.ComposeFileID)
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, &domain.ValidationError{Message: "link this compose file to at least one project before creating a vault-backed secret variable"}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })

	var canonicalPath string
	for i, project := range projects {
		path := fmt.Sprintf("secret/data/fleet/compose-files/%s/%s/%s", project.ID, in.ComposeFileID, in.Key)
		if i == 0 {
			canonicalPath = path
		}
		if err := s.secretWriter.WriteValue(ctx, in.OrganizationID, in.MountID, path, in.Value); err != nil {
			return nil, err
		}
	}

	variable, err := domain.NewVariableWithSecretRef(in.OrganizationID, in.ComposeFileID, in.Key, in.MountID, canonicalPath)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, in.RequestingUserID, variable); err != nil {
		return nil, err
	}
	return variable, nil
}
