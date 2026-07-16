package application

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"platform-of-platform/internal/identity/domain"
)

type CreateUserInput struct {
	Username   string
	Email      string
	AuthSource domain.AuthSource
	// Password is plaintext, in memory only for the duration of this
	// call - required when AuthSource is local, ignored (never hashed
	// or stored) otherwise. Hashing happens here, in /application, not
	// in /domain (docs/architecture/18-backend-structure.md §2's "pure
	// Go" rule for /domain means bcrypt can't live there).
	Password string
}

// CreateUserService implements `POST /api/v1/users` - not in Stage 4's
// resource-path list under an org, deliberately: User is platform-global
// (docs/architecture/03-domain-model.md §3), so it lives at the API root
// alongside orgs, not nested under one. Route is behind
// httpserver.RequireAuth (cmd/control-plane/main.go) - it used to be the
// one mutating route in this codebase left open, a real gap only closed
// once the admin-panel "create user & add to org" flow gave it its
// first actual caller. Deliberately no organization:manage (or any
// other org-scoped) check here - creating a User is org-independent by
// design; the real authorization boundary is the very next call in that
// flow, POST /orgs/{id}/members, which does check organization:manage.
// Real provisioning (OIDC first-login, admin invite) is still Stage
// 11/13 territory, not built here.
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

	if in.AuthSource == domain.AuthSourceLocal {
		if in.Password == "" {
			return nil, &domain.ValidationError{Message: "password is required for local auth_source"}
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		if err := user.SetPasswordHash(string(hash)); err != nil {
			return nil, err
		}
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}
