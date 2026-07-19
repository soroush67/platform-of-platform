-- Per-organization member blocking (operator's own scoped ask: suspend
-- a member's access to *this* organization only - they stay a real
-- platform User and keep working in any other org they belong to, not
-- a platform-wide account suspension). A nullable timestamp, not a bare
-- boolean - same "real state, not just a flag" shape
-- organizations.archived_at already established: NULL means never
-- blocked, a real timestamp is both the block flag and a genuine record
-- of when it happened, for free.
ALTER TABLE organization_memberships ADD COLUMN blocked_at timestamptz;
