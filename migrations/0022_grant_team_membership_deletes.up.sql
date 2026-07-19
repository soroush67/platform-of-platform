-- Team rename/delete and org-membership removal (this session's own new
-- endpoints) are the first things that ever need to hard-DELETE rows
-- from these three tables - same class of bug 0009/0013/0014/0021's own
-- comments already document hitting once each: RLS FORCE + this
-- codebase's default grants (0001_init.up.sql, 0012_teams_and_org_archival.up.sql)
-- are SELECT/INSERT/UPDATE only, never DELETE, until a real caller
-- actually needs one.
GRANT DELETE ON teams, team_memberships, organization_memberships TO platform_app;
