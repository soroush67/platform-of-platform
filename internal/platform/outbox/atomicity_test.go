package outbox_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/platform/dbtest"
	"platform-of-platform/internal/platform/outbox"
)

// TestOutboxAtomicity_RollbackDropsBothRowAndEvent is
// docs/architecture/20-testing-strategy.md's own named Outbox atomicity
// test. Proves the actual guarantee outbox.Write's own doc comment
// claims: a domain row and its outbox event commit or roll back
// together, in the same real transaction, against a real CockroachDB
// cluster - not asserted from reading the code, exercised.
func TestOutboxAtomicity_RollbackDropsBothRowAndEvent(t *testing.T) {
	ctx := context.Background()
	root := dbtest.RootPool(t)

	orgID := uuid.NewString()
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'outbox-atomicity-org', $2)`, orgID, "outbox-atomicity-org-"+orgID[:8])
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, orgID)
		mustExec(t, root, `DELETE FROM projects WHERE organization_id = $1`, orgID)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	projectID := uuid.NewString()

	// The rollback case: insert a real row and a real outbox event in
	// the same transaction as any real repository's Create() would, then
	// roll back instead of committing - simulating the exact "the
	// subsequent step in this transaction failed" scenario that's the
	// whole reason this pattern needs to be atomic in the first place.
	tx, err := root.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO projects (id, organization_id, name, slug) VALUES ($1, $2, 'atomicity-test-project', 'atomicity-test-project')`,
		projectID, orgID,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := outbox.Write(ctx, tx, orgID, "AtomicityTestRollback", map[string]any{"target_id": projectID}); err != nil {
		t.Fatalf("outbox.Write: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var projectCount, eventCount int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM projects WHERE id = $1`, projectID).Scan(&projectCount); err != nil {
		t.Fatalf("query project: %v", err)
	}
	if err := root.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE organization_id = $1 AND event_type = 'AtomicityTestRollback'`, orgID).Scan(&eventCount); err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if projectCount != 0 {
		t.Errorf("expected the rolled-back project row to not exist, found %d", projectCount)
	}
	if eventCount != 0 {
		t.Errorf("expected the rolled-back outbox event to not exist, found %d", eventCount)
	}

	// The positive case, in the same test: prove this isn't "atomicity"
	// by virtue of neither write ever actually working - a real commit
	// must persist BOTH together.
	projectID2 := uuid.NewString()
	tx2, err := root.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx2.Exec(ctx,
		`INSERT INTO projects (id, organization_id, name, slug) VALUES ($1, $2, 'atomicity-test-project-2', 'atomicity-test-project-2')`,
		projectID2, orgID,
	); err != nil {
		t.Fatalf("insert project 2: %v", err)
	}
	if err := outbox.Write(ctx, tx2, orgID, "AtomicityTestCommit", map[string]any{"target_id": projectID2}); err != nil {
		t.Fatalf("outbox.Write 2: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := root.QueryRow(ctx, `SELECT count(*) FROM projects WHERE id = $1`, projectID2).Scan(&projectCount); err != nil {
		t.Fatalf("query project 2: %v", err)
	}
	if err := root.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE organization_id = $1 AND event_type = 'AtomicityTestCommit'`, orgID).Scan(&eventCount); err != nil {
		t.Fatalf("query outbox_events 2: %v", err)
	}
	if projectCount != 1 {
		t.Errorf("expected the committed project row to exist exactly once, found %d", projectCount)
	}
	if eventCount != 1 {
		t.Errorf("expected the committed outbox event to exist exactly once, found %d", eventCount)
	}
}

// TestOutboxRelay_PublishesCommittedEvent proves the consumption side
// too, not just the write-side atomicity above - a real outbox.Relay,
// running against the real cluster, actually picks up a freshly
// committed event and marks it published (the mechanism every real
// consumer in this codebase, Audit and the Run Dispatcher, depends on).
func TestOutboxRelay_PublishesCommittedEvent(t *testing.T) {
	ctx := context.Background()
	root := dbtest.RootPool(t)

	orgID := uuid.NewString()
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'outbox-relay-org', $2)`, orgID, "outbox-relay-org-"+orgID[:8])
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, orgID)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	tx, err := root.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := outbox.Write(ctx, tx, orgID, "RelayTestEvent", map[string]any{"hello": "world"}); err != nil {
		t.Fatalf("outbox.Write: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	received := make(chan outbox.Event, 1)
	handler := func(_ context.Context, event outbox.Event) error {
		if event.OrganizationID == orgID {
			received <- event
		}
		return nil
	}

	relay := outbox.NewRelay(root, handler, 100*time.Millisecond, slog.Default())
	relayCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	go relay.Run(relayCtx)

	select {
	case event := <-received:
		if event.EventType != "RelayTestEvent" {
			t.Errorf("expected event_type RelayTestEvent, got %q", event.EventType)
		}
	case <-relayCtx.Done():
		t.Fatal("timed out waiting for the Relay to pick up a real, committed outbox event")
	}

	var publishedAt *time.Time
	if err := root.QueryRow(ctx, `SELECT published_at FROM outbox_events WHERE organization_id = $1 AND event_type = 'RelayTestEvent'`, orgID).Scan(&publishedAt); err != nil {
		t.Fatalf("query published_at: %v", err)
	}
	if publishedAt == nil {
		t.Error("expected published_at to be set after the Relay processed the event, still NULL")
	}
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
