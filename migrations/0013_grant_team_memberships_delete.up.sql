-- Real bug, found by actually calling DELETE /teams/{team}/members/{user_id}
-- against the running stack, not assumed: 0012_teams_and_org_archival.up.sql
-- granted SELECT/INSERT/UPDATE on team_memberships but not DELETE -
-- RemoveTeamMemberService's DELETE FROM team_memberships failed with
-- "user platform_app does not have DELETE privilege," the exact same
-- class of bug migration 0009 fixed for role_bindings earlier this
-- project.
GRANT DELETE ON team_memberships TO platform_app;
