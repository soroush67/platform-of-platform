package application_test

import (
	"context"
	"sync"

	"platform-of-platform/internal/identity/domain"
)

type fakeUserRepo struct {
	mu    sync.Mutex
	byID  map[string]*domain.User
	byErr error
}

func newFakeUserRepo() *fakeUserRepo { return &fakeUserRepo{byID: map[string]*domain.User{}} }

func (f *fakeUserRepo) Create(ctx context.Context, u *domain.User) error {
	if f.byErr != nil {
		return f.byErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *u
	f.byID[u.ID] = &cp
	return nil
}

func (f *fakeUserRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.byID {
		if u.Username == username {
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	cp := *u
	return &cp, nil
}

func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.byID {
		if u.Email == email {
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeUserRepo) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.PasswordHash = &passwordHash
	return nil
}

func (f *fakeUserRepo) put(u *domain.User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *u
	f.byID[u.ID] = &cp
}

type fakeRefreshTokenRepo struct {
	mu     sync.Mutex
	tokens map[string]*domain.RefreshToken // by hash
}

func newFakeRefreshTokenRepo() *fakeRefreshTokenRepo {
	return &fakeRefreshTokenRepo{tokens: map[string]*domain.RefreshToken{}}
}

func (f *fakeRefreshTokenRepo) Create(ctx context.Context, t *domain.RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *t
	f.tokens[t.TokenHash] = &cp
	return nil
}

func (f *fakeRefreshTokenRepo) GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tokens[tokenHash]
	if !ok {
		return nil, domain.ErrRefreshTokenInvalid
	}
	cp := *t
	return &cp, nil
}

func (f *fakeRefreshTokenRepo) Revoke(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.ID == id {
			now := t.CreatedAt
			t.RevokedAt = &now
		}
	}
	return nil
}

type fakePasswordResetTokenRepo struct {
	mu     sync.Mutex
	tokens map[string]*domain.PasswordResetToken
}

func newFakePasswordResetTokenRepo() *fakePasswordResetTokenRepo {
	return &fakePasswordResetTokenRepo{tokens: map[string]*domain.PasswordResetToken{}}
}

func (f *fakePasswordResetTokenRepo) Create(ctx context.Context, t *domain.PasswordResetToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *t
	f.tokens[t.TokenHash] = &cp
	return nil
}

func (f *fakePasswordResetTokenRepo) GetByHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tokens[tokenHash]
	if !ok {
		return nil, domain.ErrPasswordResetTokenInvalid
	}
	cp := *t
	return &cp, nil
}

func (f *fakePasswordResetTokenRepo) MarkUsed(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.ID == id {
			now := t.CreatedAt
			t.UsedAt = &now
		}
	}
	return nil
}

type fakeServiceAccountRepo struct {
	mu   sync.Mutex
	byID map[string]*domain.ServiceAccount
}

func newFakeServiceAccountRepo() *fakeServiceAccountRepo {
	return &fakeServiceAccountRepo{byID: map[string]*domain.ServiceAccount{}}
}

func (f *fakeServiceAccountRepo) Create(ctx context.Context, sa *domain.ServiceAccount) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *sa
	f.byID[sa.ID] = &cp
	return nil
}

func (f *fakeServiceAccountRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.ServiceAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sa, ok := f.byID[id]
	if !ok || sa.OrganizationID != organizationID {
		return nil, domain.ErrServiceAccountNotFound
	}
	cp := *sa
	return &cp, nil
}

type fakeAPIKeyRepo struct {
	mu      sync.Mutex
	byHash  map[string]*domain.APIKey
	orgOf   map[string]string // key id -> organization id
	revoked map[string]bool
	touched []string
}

func newFakeAPIKeyRepo() *fakeAPIKeyRepo {
	return &fakeAPIKeyRepo{byHash: map[string]*domain.APIKey{}, orgOf: map[string]string{}, revoked: map[string]bool{}}
}

func (f *fakeAPIKeyRepo) Create(ctx context.Context, organizationID string, key *domain.APIKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *key
	f.byHash[key.KeyHash] = &cp
	f.orgOf[key.ID] = organizationID
	return nil
}

func (f *fakeAPIKeyRepo) GetByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, ok := f.byHash[keyHash]
	if !ok {
		return nil, domain.ErrAPIKeyInvalid
	}
	cp := *k
	if f.revoked[k.ID] {
		now := k.CreatedAt
		cp.RevokedAt = &now
	}
	return &cp, nil
}

func (f *fakeAPIKeyRepo) TouchLastUsed(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeAPIKeyRepo) Revoke(ctx context.Context, organizationID, keyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.orgOf[keyID] != organizationID {
		return domain.ErrAPIKeyInvalid
	}
	if f.revoked[keyID] {
		return domain.ErrAPIKeyInvalid
	}
	f.revoked[keyID] = true
	return nil
}

type fakeMembershipChecker struct {
	mu      sync.Mutex
	members map[string]bool
}

func newFakeMembershipChecker() *fakeMembershipChecker {
	return &fakeMembershipChecker{members: map[string]bool{}}
}

func (f *fakeMembershipChecker) IsMember(ctx context.Context, organizationID, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.members[organizationID+"|"+userID], nil
}

func (f *fakeMembershipChecker) add(orgID, userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[orgID+"|"+userID] = true
}

type fakePermissionChecker struct {
	mu    sync.Mutex
	perms map[string]bool
}

func newFakePermissionChecker() *fakePermissionChecker {
	return &fakePermissionChecker{perms: map[string]bool{}}
}

func (f *fakePermissionChecker) HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.perms[organizationID+"|"+userID+"|"+permission], nil
}

func (f *fakePermissionChecker) grant(orgID, userID, permission string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.perms[orgID+"|"+userID+"|"+permission] = true
}
