-- Closes the "RoleBinding is pure-additive-OR only, no explicit-deny"
-- gap named directly - docs/architecture/03-domain-model.md §4's own
-- "a binding at a higher scope implies the grant... unless a more
-- specific binding narrows it" needed a real deny concept to actually
-- mean anything; there was none until now. AWS-IAM-style evaluation
-- (explicit deny always wins over any allow, regardless of which scope
-- each came from), not Kubernetes RBAC's pure-additive model - see
-- internal/rbac/adapters/postgres/role_binding_repository.go's own
-- comment on why deny-overrides-allow is the real fix for "narrowing."
ALTER TABLE role_bindings
    ADD COLUMN effect text NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny'));
