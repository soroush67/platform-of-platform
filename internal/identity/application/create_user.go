package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
)

type CreateUserInput struct {
	Username   string
	Email      string
	AuthSource domain.AuthSource
}

// CreateUserService implements `POST /api/v1/users` - not in Stage 4's
// resource-path list under an org, deliberately: User is platform-global
// (docs/architecture/03-domain-model.md §3), so it lives at the API root
// alongside orgs, not nested under one. Deliberately unauthenticated for
// now, same posture as CreateOrganizationService - real provisioning
// (OIDC first-login, admin invite) is Stage 11/13 territory, not this
// walking skeleton's concern yet.
type CreateUserService struct {
	repo UserRepository
}

func NewCreateUserService(repo UserRepository) *CreateUserService {
	return &CreateUserService{repo: repo}
}

func (s *CreateUserService) Execute(ctx context.Context, in CreateUserInput) (*domain.User, error) {
	user, err := domain.NewUser(in.Username, in.Email, in.AuthSource)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}
