// Package vault is the Secrets context's real backend adapter -
// docs/architecture/11-module-secrets-state.md §1's only actually-
// implemented backend_type (see secrets/domain.BackendType's own doc
// comment on why the other three are modeled but not wired to a real
// adapter). Uses the real HashiCorp Vault Go SDK
// (github.com/hashicorp/vault/api) against a real Vault server
// (docker-compose's own `vault` dev-mode service) - no mock, no fake
// backend.
package vault

import (
	"context"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
)

// secretValueKey is the one key this adapter reads out of a KV v2
// secret's data map. Vault's KV v2 stores an arbitrary map at a path,
// but SecretReference (docs/architecture/11-module-secrets-state.md §2)
// models one Variable pointing at one path for one scalar value - a
// real, deliberate convention this adapter enforces rather than
// returning a nested map through Variables' own single-string
// ResolveValue port. An operator writing a secret for this platform to
// consume writes it as {"value": "..."} at their chosen path, the same
// "one path, one key, one value" shape Terraform Cloud's own sensitive
// variables use.
const secretValueKey = "value"

type Client struct{}

func NewClient() *Client { return &Client{} }

func (c *Client) login(ctx context.Context, address, roleID, secretID string) (*vaultapi.Client, error) {
	cfg := vaultapi.DefaultConfig()
	cfg.Address = address
	client, err := vaultapi.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("vault: constructing client for %s: %w", address, err)
	}

	// Real AppRole login (docs/architecture/11-module-secrets-state.md
	// §1) - a raw Logical().Write against auth/approle/login rather than
	// the SDK's separate auth/approle helper module, avoiding an extra
	// dependency for what's a two-field POST.
	secret, err := client.Logical().WriteWithContext(ctx, "auth/approle/login", map[string]any{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		return nil, fmt.Errorf("vault: approle login failed: %w", err)
	}
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return nil, fmt.Errorf("vault: approle login returned no client token")
	}

	client.SetToken(secret.Auth.ClientToken)
	return client, nil
}

// TestConnection implements application.VaultClient - a real AppRole
// login (proves address/role_id/secret_id are all genuinely valid and
// the server is reachable) followed by a real, content-free "is this
// token actually live" check - never reads or returns any secret
// content, matching the doc's own "without revealing any secret
// content" requirement for this endpoint.
func (c *Client) TestConnection(ctx context.Context, address, roleID, secretID string) error {
	client, err := c.login(ctx, address, roleID, secretID)
	if err != nil {
		return err
	}
	if _, err := client.Auth().Token().LookupSelfWithContext(ctx); err != nil {
		return fmt.Errorf("vault: token lookup failed: %w", err)
	}
	return nil
}

// ReadSecret implements application.VaultClient - a real AppRole login
// followed by a real KV v2 read at path (the caller's own full data
// path, e.g. "secret/data/database/prod/password" - this adapter
// doesn't prepend or rewrite it).
func (c *Client) ReadSecret(ctx context.Context, address, roleID, secretID, path string) (string, error) {
	client, err := c.login(ctx, address, roleID, secretID)
	if err != nil {
		return "", err
	}

	secret, err := client.Logical().ReadWithContext(ctx, path)
	if err != nil {
		return "", fmt.Errorf("vault: reading %q: %w", path, err)
	}
	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("vault: no secret found at %q", path)
	}

	// KV v2's own response shape wraps the real payload one level down,
	// under "data" (the outer secret.Data is KV v2's envelope - "data"
	// + "metadata" - not the payload itself).
	inner, ok := secret.Data["data"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("vault: %q is not a KV v2 secret (no nested \"data\" map - is this mount KV v1?)", path)
	}
	value, ok := inner[secretValueKey].(string)
	if !ok {
		return "", fmt.Errorf("vault: secret at %q has no string %q key", path, secretValueKey)
	}
	return value, nil
}
