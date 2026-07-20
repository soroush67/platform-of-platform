package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/outbox"
)

type AttachmentRepository struct {
	pool *pgxpool.Pool
}

func NewAttachmentRepository(pool *pgxpool.Pool) *AttachmentRepository {
	return &AttachmentRepository{pool: pool}
}

func (r *AttachmentRepository) AttachNetwork(ctx context.Context, actorUserID, organizationID, composeFileID, networkID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	// ON CONFLICT DO NOTHING - re-attaching an already-attached Network
	// is a safe no-op, matching the ported Python product's own 409-on-
	// duplicate being a client-visible concern the HTTP handler decides,
	// not a hard repository error either way is acceptable for; this
	// codebase's own idempotency-friendly posture (e.g. AddMemberService)
	// favors the no-op here.
	_, err = tx.Exec(ctx,
		`INSERT INTO compose_file_networks (id, organization_id, compose_file_id, network_id) VALUES (gen_random_uuid(), $1, $2, $3)
		 ON CONFLICT (compose_file_id, network_id) DO NOTHING`,
		organizationID, composeFileID, networkID,
	)
	if err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetNetworkAttached", map[string]any{
		"actor": actorUserID, "target_type": "compose_file", "target_id": composeFileID, "network_id": networkID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *AttachmentRepository) DetachNetwork(ctx context.Context, actorUserID, organizationID, composeFileID, networkID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM compose_file_networks WHERE organization_id = $1 AND compose_file_id = $2 AND network_id = $3`,
		organizationID, composeFileID, networkID,
	); err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetNetworkDetached", map[string]any{
		"actor": actorUserID, "target_type": "compose_file", "target_id": composeFileID, "network_id": networkID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *AttachmentRepository) ListNetworksForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]*domain.Network, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT n.id, n.organization_id, n.name, n.external, n.created_by, n.created_at
		 FROM networks n JOIN compose_file_networks cfn ON cfn.network_id = n.id
		 WHERE cfn.organization_id = $1 AND cfn.compose_file_id = $2 ORDER BY n.name`,
		organizationID, composeFileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var networks []*domain.Network
	for rows.Next() {
		n, err := scanNetwork(rows)
		if err != nil {
			return nil, err
		}
		networks = append(networks, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return networks, tx.Commit(ctx)
}

func (r *AttachmentRepository) AttachVolume(ctx context.Context, actorUserID, organizationID, composeFileID, volumeID, containerPath string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO compose_file_volumes (id, organization_id, compose_file_id, volume_id, container_path) VALUES (gen_random_uuid(), $1, $2, $3, $4)
		 ON CONFLICT (compose_file_id, volume_id) DO UPDATE SET container_path = EXCLUDED.container_path`,
		organizationID, composeFileID, volumeID, containerPath,
	)
	if err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetVolumeAttached", map[string]any{
		"actor": actorUserID, "target_type": "compose_file", "target_id": composeFileID, "volume_id": volumeID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *AttachmentRepository) DetachVolume(ctx context.Context, actorUserID, organizationID, composeFileID, volumeID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM compose_file_volumes WHERE organization_id = $1 AND compose_file_id = $2 AND volume_id = $3`,
		organizationID, composeFileID, volumeID,
	); err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetVolumeDetached", map[string]any{
		"actor": actorUserID, "target_type": "compose_file", "target_id": composeFileID, "volume_id": volumeID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *AttachmentRepository) ListVolumesForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]application.VolumeAttachmentView, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT v.id, v.organization_id, v.name, v.host_path, v.created_by, v.created_at, cfv.container_path
		 FROM volumes v JOIN compose_file_volumes cfv ON cfv.volume_id = v.id
		 WHERE cfv.organization_id = $1 AND cfv.compose_file_id = $2 ORDER BY v.name`,
		organizationID, composeFileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []application.VolumeAttachmentView
	for rows.Next() {
		var v domain.Volume
		var containerPath string
		if err := rows.Scan(&v.ID, &v.OrganizationID, &v.Name, &v.HostPath, &v.CreatedBy, &v.CreatedAt, &containerPath); err != nil {
			return nil, err
		}
		views = append(views, application.VolumeAttachmentView{Volume: &v, ContainerPath: containerPath})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return views, tx.Commit(ctx)
}

func (r *AttachmentRepository) AttachProject(ctx context.Context, actorUserID, organizationID, composeFileID, projectID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO compose_file_projects (id, organization_id, compose_file_id, project_id) VALUES (gen_random_uuid(), $1, $2, $3)
		 ON CONFLICT (compose_file_id, project_id) DO NOTHING`,
		organizationID, composeFileID, projectID,
	)
	if err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetComposeFileProjectAttached", map[string]any{
		"actor": actorUserID, "target_type": "compose_file", "target_id": composeFileID, "project_id": projectID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *AttachmentRepository) DetachProject(ctx context.Context, actorUserID, organizationID, composeFileID, projectID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM compose_file_projects WHERE organization_id = $1 AND compose_file_id = $2 AND project_id = $3`,
		organizationID, composeFileID, projectID,
	); err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetComposeFileProjectDetached", map[string]any{
		"actor": actorUserID, "target_type": "compose_file", "target_id": composeFileID, "project_id": projectID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ListComposeFilesForProject is ListProjectsForComposeFile's reverse -
// joins compose_file_projects the other way, reusing the same
// composeFileSelectColumns/scanComposeFile helpers ComposeFileRepository
// itself defines (same package, both compiled from internal/fleet/
// adapters/postgres).
func (r *AttachmentRepository) ListComposeFilesForProject(ctx context.Context, organizationID, projectID string) ([]*domain.ComposeFile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT cf.id, cf.organization_id, cf.name, cf.is_global, cf.compose_content, cf.created_by, cf.created_at
		 FROM compose_files cf JOIN compose_file_projects cfp ON cfp.compose_file_id = cf.id
		 WHERE cfp.organization_id = $1 AND cfp.project_id = $2 ORDER BY cf.name`,
		organizationID, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*domain.ComposeFile
	for rows.Next() {
		c, err := scanComposeFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, tx.Commit(ctx)
}

// ListProjectsForComposeFile joins straight into Tenancy's own projects
// table (same-DB SQL join, no cross-context Go call) - matches
// ListNetworksForComposeFile's own join into the sibling networks table;
// the only difference is projects isn't a table Fleet's own migrations
// created, but it's the same Postgres cluster and RLS already scopes it
// by the same app.current_org_id this transaction just set.
func (r *AttachmentRepository) ListProjectsForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]application.ProjectSummary, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT p.id, p.name, p.slug
		 FROM projects p JOIN compose_file_projects cfp ON cfp.project_id = p.id
		 WHERE cfp.organization_id = $1 AND cfp.compose_file_id = $2 ORDER BY p.name`,
		organizationID, composeFileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []application.ProjectSummary
	for rows.Next() {
		var p application.ProjectSummary
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug); err != nil {
			return nil, err
		}
		summaries = append(summaries, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return summaries, tx.Commit(ctx)
}
