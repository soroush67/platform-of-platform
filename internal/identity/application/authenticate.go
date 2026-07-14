package application

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"platform-of-platform/internal/identity/domain"
)

// AuthenticateService implements local-auth login
// (docs/architecture/04-api-design.md §4's "User session ... local
// login" credential type; docs/architecture/19-integrations.md §2
// explains why OIDC/SAML delegate entirely instead of going through
// this path). Returns domain.ErrInvalidCredentials for every failure
// mode - unknown username, wrong password, or a non-local user
// attempting password login - deliberately the same error for all
// three, so a login form can never be used to enumerate which
// usernames exist (the same "don't reveal existence" posture already
// applied to org lookups).
type AuthenticateService struct {
	repo UserRepository
}

func NewAuthenticateService(repo UserRepository) *AuthenticateService {
	return &AuthenticateService{repo: repo}
}

func (s *AuthenticateService) Execute(ctx context.Context, username, password string) (*domain.User, error) {
	user, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}

	if user.AuthSource != domain.AuthSourceLocal || user.PasswordHash == nil {
		return nil, domain.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	return user, nil
}
