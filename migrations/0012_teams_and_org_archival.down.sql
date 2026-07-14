DROP TABLE IF EXISTS team_memberships;
DROP TABLE IF EXISTS teams;
ALTER TABLE organizations DROP COLUMN IF EXISTS archived_at;
ALTER TABLE organizations DROP COLUMN IF EXISTS status;
