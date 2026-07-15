package application_test

import (
	"context"
	"testing"

	"platform-of-platform/internal/audit/application"
	"platform-of-platform/internal/platform/outbox"
)

func TestRecordEntryService_TurnsAnOutboxEventIntoAnAuditEntry(t *testing.T) {
	repo := newFakeAuditEntryRepo()
	svc := application.NewRecordEntryService(repo)

	event := outbox.Event{
		ID:             "event-1",
		OrganizationID: testOrgID,
		EventType:      "OrganizationCreated",
		Payload:        []byte(`{"actor":"user-1","target_type":"organization","target_id":"org-1","name":"Acme"}`),
	}

	if err := svc.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	page, err := repo.ListByOrganization(context.Background(), testOrgID, 10, nil, nil)
	if err != nil {
		t.Fatalf("ListByOrganization: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("expected exactly 1 recorded entry, got %d", len(page))
	}
	entry := page[0]
	if entry.SourceEventID != "event-1" {
		t.Errorf("expected SourceEventID to tie back to the outbox event id, got %q", entry.SourceEventID)
	}
	if entry.Actor != "user-1" || entry.TargetType != "organization" || entry.TargetID != "org-1" {
		t.Errorf("expected actor/target fields pulled from the payload, got %+v", entry)
	}
	if entry.Action != "OrganizationCreated" {
		t.Errorf("expected Action to be the event type, got %q", entry.Action)
	}
	if entry.Metadata["name"] != "Acme" {
		t.Errorf("expected the rest of the payload to become metadata as-is, got %+v", entry.Metadata)
	}
}

func TestRecordEntryService_MalformedPayloadIsRejected(t *testing.T) {
	repo := newFakeAuditEntryRepo()
	svc := application.NewRecordEntryService(repo)

	event := outbox.Event{ID: "event-2", OrganizationID: testOrgID, EventType: "SomeEvent", Payload: []byte(`not json`)}

	if err := svc.HandleEvent(context.Background(), event); err == nil {
		t.Fatal("expected an error for a malformed event payload")
	}
}
