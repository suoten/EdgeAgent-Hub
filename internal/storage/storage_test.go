package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := New(dbPath, true, 5, 7, 180, 1000)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestAlertCRUD(t *testing.T) {
	store := setupTestStore(t)

	alert := &models.Alert{
		ID:         "alert-001",
		WorkflowID: "wf-test-001",
		DeviceID:   "line1.motor2",
		Type:       "vibration_anomaly",
		Severity:   models.AlertSeverityHigh,
		Confidence: 0.92,
		Message:    "振动异常",
		Anchored:   true,
		Evidence: models.AlertEvidence{
			SensorValue: 7.2,
			Threshold:   4.5,
		},
		Status:    models.AlertStatusActive,
		CreatedAt: time.Now(),
	}

	if err := store.SaveAlert(alert); err != nil {
		t.Fatalf("SaveAlert failed: %v", err)
	}

	alerts, err := store.GetAlerts("active", 10, 0)
	if err != nil {
		t.Fatalf("GetAlerts failed: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].ID != "alert-001" {
		t.Errorf("expected ID alert-001, got %s", alerts[0].ID)
	}
	if !alerts[0].Anchored {
		t.Error("expected anchored=true")
	}

	// Ack
	if err := store.AckAlert("alert-001", "admin"); err != nil {
		t.Fatalf("AckAlert failed: %v", err)
	}

	alerts, _ = store.GetAlerts("active", 10, 0)
	if len(alerts) != 0 {
		t.Errorf("expected 0 active alerts after ack, got %d", len(alerts))
	}
}

func TestAuditLog(t *testing.T) {
	store := setupTestStore(t)

	entry := &models.AuditEntry{
		Timestamp: time.Now(),
		Actor:     "admin",
		Action:    "model:load",
		Resource:  "vibration_v1",
		IP:        "127.0.0.1",
	}

	if err := store.SaveAudit(entry); err != nil {
		t.Fatalf("SaveAudit failed: %v", err)
	}

	logs, err := store.GetAuditLogs(10, 0, nil, nil)
	if err != nil {
		t.Fatalf("GetAuditLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Actor != "admin" {
		t.Errorf("expected actor admin, got %s", logs[0].Actor)
	}
}

func TestOfflineQueue(t *testing.T) {
	store := setupTestStore(t)

	// Enqueue
	if err := store.EnqueueOffline("test.subject", []byte("payload1")); err != nil {
		t.Fatalf("EnqueueOffline failed: %v", err)
	}
	if err := store.EnqueueOffline("test.subject2", []byte("payload2")); err != nil {
		t.Fatalf("EnqueueOffline failed: %v", err)
	}

	// Check length
	length, err := store.OfflineQueueLength()
	if err != nil {
		t.Fatalf("OfflineQueueLength failed: %v", err)
	}
	if length != 2 {
		t.Errorf("expected length 2, got %d", length)
	}

	// Dequeue
	msgs, err := store.DequeueOffline(10)
	if err != nil {
		t.Fatalf("DequeueOffline failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Mark sent
	if err := store.MarkOfflineSent(msgs[0].ID); err != nil {
		t.Fatalf("MarkOfflineSent failed: %v", err)
	}

	length, _ = store.OfflineQueueLength()
	if length != 1 {
		t.Errorf("expected length 1 after sent, got %d", length)
	}
}

func TestUserCRUD(t *testing.T) {
	store := setupTestStore(t)

	user := &models.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$fakehash",
		Role:         models.RoleOperator,
		CreatedAt:    time.Now(),
	}

	if err := store.SaveUser(user); err != nil {
		t.Fatalf("SaveUser failed: %v", err)
	}

	// Get
	retrieved, err := store.GetUser("testuser")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if retrieved.Role != models.RoleOperator {
		t.Errorf("expected role operator, got %s", retrieved.Role)
	}

	// List
	users, err := store.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
}

func TestWorkflowCRUD(t *testing.T) {
	store := setupTestStore(t)

	definition := []byte(`{"name":"test","steps":[]}`)
	if err := store.SaveWorkflow("test", definition); err != nil {
		t.Fatalf("SaveWorkflow failed: %v", err)
	}

	workflows, err := store.ListWorkflows()
	if err != nil {
		t.Fatalf("ListWorkflows failed: %v", err)
	}
	if len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(workflows))
	}

	// Get
	data, err := store.GetWorkflow("test")
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if string(data) != `{"name":"test","steps":[]}` {
		t.Errorf("unexpected workflow data: %s", string(data))
	}

	// Delete
	if err := store.DeleteWorkflow("test"); err != nil {
		t.Fatalf("DeleteWorkflow failed: %v", err)
	}

	workflows, _ = store.ListWorkflows()
	if len(workflows) != 0 {
		t.Errorf("expected 0 workflows after delete, got %d", len(workflows))
	}
}

func TestTSData(t *testing.T) {
	store := setupTestStore(t)

	// Save
	for i := 0; i < 5; i++ {
		store.SaveTSData("device1", "temperature", 60.0+float64(i), int64(i), "good")
	}

	// Get
	data, err := store.GetTSData("device1", "temperature", 3)
	if err != nil {
		t.Fatalf("GetTSData failed: %v", err)
	}
	if len(data) != 3 {
		t.Fatalf("expected 3 data points, got %d", len(data))
	}
}

func TestLoginAttempts(t *testing.T) {
	store := setupTestStore(t)

	store.RecordLoginAttempt("user1", "127.0.0.1", false)
	store.RecordLoginAttempt("user1", "127.0.0.1", false)
	store.RecordLoginAttempt("user1", "127.0.0.1", true)

	count, err := store.GetFailedLoginCount("user1", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("GetFailedLoginCount failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 failed attempts, got %d", count)
	}
}
