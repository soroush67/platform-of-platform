-- The Purge Reaper (internal/tenancy/application/purge_reaper.go) is
-- the first thing in this codebase that ever needs to hard-DELETE rows
-- from most of these tables - every previous "delete" was either a soft
-- delete (status flip) or one of the two narrow cases already granted
-- (role_bindings: migration 0009, team_memberships: migration 0013).
-- Found for real by actually running the reaper against an archived
-- org: it failed with "user platform_app does not have DELETE privilege
-- on relation runs" - the exact same class of bug as 0009 and 0013,
-- just against a much longer list of tables this time.
GRANT DELETE ON runs, outbox_events, idempotency_keys, variables, workspaces,
  environments, teams, roles, projects, organization_memberships,
  audit_entries, organizations TO platform_app;
