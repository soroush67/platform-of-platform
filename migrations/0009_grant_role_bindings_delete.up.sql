-- ChangeMemberRoleService's ReplaceRole (internal/rbac/adapters/postgres/
-- role_binding_repository.go) needs to DELETE an existing organization-
-- scope binding before inserting the replacement - 0001_init.up.sql
-- only ever granted SELECT/INSERT/UPDATE on role_bindings, since nothing
-- needed to delete a binding until real role-change semantics existed.
GRANT DELETE ON role_bindings TO platform_app;
