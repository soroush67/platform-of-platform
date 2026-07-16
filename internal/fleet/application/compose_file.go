package application

import (
	"context"

	"platform-of-platform/internal/fleet/domain"
)

type CreateComposeFileInput struct {
	OrganizationID   string
	RequestingUserID string
	Name             string
	ComposeContent   string
	IsGlobal         bool
}

type CreateComposeFileService struct {
	repo        ComposeFileRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewCreateComposeFileService(repo ComposeFileRepository, membership MembershipChecker, permChecker PermissionChecker) *CreateComposeFileService {
	return &CreateComposeFileService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *CreateComposeFileService) Execute(ctx context.Context, in CreateComposeFileInput) (*domain.ComposeFile, error) {
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

	if err := ValidateComposeContent(in.ComposeContent); err != nil {
		return nil, err
	}

	composeFile, err := domain.NewComposeFile(in.OrganizationID, in.Name, in.ComposeContent, in.RequestingUserID, in.IsGlobal)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, composeFile); err != nil {
		return nil, err
	}
	return composeFile, nil
}

type ListComposeFilesService struct {
	repo        ComposeFileRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewListComposeFilesService(repo ComposeFileRepository, membership MembershipChecker, permChecker PermissionChecker) *ListComposeFilesService {
	return &ListComposeFilesService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *ListComposeFilesService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.ComposeFile, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionComposeFileRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListByOrganization(ctx, organizationID)
}

type GetComposeFileService struct {
	repo        ComposeFileRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewGetComposeFileService(repo ComposeFileRepository, membership MembershipChecker, permChecker PermissionChecker) *GetComposeFileService {
	return &GetComposeFileService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *GetComposeFileService) Execute(ctx context.Context, organizationID, requestingUserID, composeFileID string) (*domain.ComposeFile, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionComposeFileRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}
	return s.repo.GetByID(ctx, organizationID, composeFileID)
}

type UpdateComposeFileContentService struct {
	repo        ComposeFileRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewUpdateComposeFileContentService(repo ComposeFileRepository, membership MembershipChecker, permChecker PermissionChecker) *UpdateComposeFileContentService {
	return &UpdateComposeFileContentService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *UpdateComposeFileContentService) Execute(ctx context.Context, organizationID, requestingUserID, composeFileID, content string) error {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionComposeFileManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	if err := ValidateComposeContent(content); err != nil {
		return err
	}

	return s.repo.UpdateContent(ctx, requestingUserID, organizationID, composeFileID, content)
}
