package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/edgelite/edgeagent-hub/internal/models"
)

// Store 是 EdgeAgent Hub 的存储层，使用 SQLite (WAL 模式)
type Store struct {
	db                 *sql.DB
	dbPath             string
	mu                 sync.RWMutex
	retentionDays      int
	auditRetentionDays int
	offlineQueueMax    int
}

// New 创建并初始化存储层
func New(dbPath string, walMode bool, maxConns, retentionDays, auditRetentionDays, offlineQueueMax int) (*Store, error) {
	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", dbPath)
	if !walMode {
		dsn = fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", dbPath)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns / 2)
	db.SetConnMaxLifetime(0)

	s := &Store{
		db:                  db,
		dbPath:              dbPath,
		retentionDays:       retentionDays,
		auditRetentionDays: auditRetentionDays,
		offlineQueueMax:     offlineQueueMax,
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return s, nil
}

// Close 关闭存储层
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate 执行数据库迁移
func (s *Store) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS alerts (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			type TEXT NOT NULL,
			severity TEXT NOT NULL,
			confidence REAL NOT NULL,
			message TEXT NOT NULL,
			anchored INTEGER NOT NULL DEFAULT 0,
			evidence TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			acked_at DATETIME,
			acked_by TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_device ON alerts(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_created ON alerts(created_at DESC)`,

		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			actor TEXT NOT NULL,
			action TEXT NOT NULL,
			resource TEXT NOT NULL,
			detail TEXT,
			ip TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor)`,

		`CREATE TABLE IF NOT EXISTS offline_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			topic TEXT NOT NULL,
			payload BLOB NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			retries INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_offline_status ON offline_queue(status)`,

		`CREATE TABLE IF NOT EXISTS users (
			username TEXT PRIMARY KEY,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_login DATETIME
		)`,

		`CREATE TABLE IF NOT EXISTS models (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			type TEXT NOT NULL,
			file_path TEXT NOT NULL,
			partition TEXT NOT NULL,
			active INTEGER NOT NULL DEFAULT 0,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			sha256 TEXT NOT NULL,
			loaded_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_models_active ON models(active)`,

		`CREATE TABLE IF NOT EXISTS ts_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			metric_name TEXT NOT NULL,
			value REAL NOT NULL,
			timestamp INTEGER NOT NULL,
			quality TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ts_device_metric_time ON ts_cache(device_id, metric_name, timestamp DESC)`,

		`CREATE TABLE IF NOT EXISTS workflows (
			name TEXT PRIMARY KEY,
			definition TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS ota_tasks (
			id TEXT PRIMARY KEY,
			model_name TEXT NOT NULL,
			version TEXT NOT NULL,
			target_partition TEXT NOT NULL,
			status TEXT NOT NULL,
			sha256 TEXT,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME,
			error TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS login_attempts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			ip TEXT NOT NULL,
			success INTEGER NOT NULL,
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_login_attempts_user_time ON login_attempts(username, timestamp DESC)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}

	return nil
}

// ── 告警操作 ──

func (s *Store) SaveAlert(alert *models.Alert) error {
	evidenceJSON, _ := json.Marshal(alert.Evidence)
	_, err := s.db.Exec(
		`INSERT INTO alerts (id, workflow_id, device_id, type, severity, confidence, message, anchored, evidence, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		alert.ID, alert.WorkflowID, alert.DeviceID, alert.Type, alert.Severity,
		alert.Confidence, alert.Message, boolToInt(alert.Anchored), string(evidenceJSON),
		alert.Status, alert.CreatedAt,
	)
	return err
}

func (s *Store) GetAlerts(status string, limit, offset int) ([]models.Alert, error) {
	query := `SELECT id, workflow_id, device_id, type, severity, confidence, message, anchored, evidence, status, created_at, acked_at, acked_by
		FROM alerts`
	var args []interface{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		var a models.Alert
		var evidenceJSON string
		var anchored int
		var ackedAt sql.NullTime
		var ackedBy sql.NullString

		if err := rows.Scan(&a.ID, &a.WorkflowID, &a.DeviceID, &a.Type, &a.Severity,
			&a.Confidence, &a.Message, &anchored, &evidenceJSON, &a.Status, &a.CreatedAt, &ackedAt, &ackedBy); err != nil {
			return nil, err
		}
		a.Anchored = anchored == 1
		json.Unmarshal([]byte(evidenceJSON), &a.Evidence)
		if ackedAt.Valid {
			a.AckedAt = &ackedAt.Time
		}
		if ackedBy.Valid {
			a.AckedBy = ackedBy.String
		}
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return alerts, nil
}

func (s *Store) AckAlert(alertID, ackedBy string) error {
	_, err := s.db.Exec(
		`UPDATE alerts SET status = 'acked', acked_at = ?, acked_by = ? WHERE id = ?`,
		time.Now(), ackedBy, alertID,
	)
	return err
}

// ── 审计日志 ──

func (s *Store) SaveAudit(entry *models.AuditEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO audit_log (timestamp, actor, action, resource, detail, ip) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.Timestamp, entry.Actor, entry.Action, entry.Resource, entry.Detail, entry.IP,
	)
	return err
}

func (s *Store) GetAuditLogs(limit, offset int, startTime, endTime *time.Time) ([]models.AuditEntry, error) {
	query := `SELECT id, timestamp, actor, action, resource, detail, ip FROM audit_log WHERE 1=1`
	var args []interface{}
	if startTime != nil {
		query += ` AND timestamp >= ?`
		args = append(args, *startTime)
	}
	if endTime != nil {
		query += ` AND timestamp <= ?`
		args = append(args, *endTime)
	}
	query += ` ORDER BY timestamp DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.AuditEntry
	for rows.Next() {
		var e models.AuditEntry
		var detail, ip sql.NullString
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Actor, &e.Action, &e.Resource, &detail, &ip); err != nil {
			return nil, err
		}
		if detail.Valid {
			e.Detail = detail.String
		}
		if ip.Valid {
			e.IP = ip.String
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// ── 离线队列 ──

func (s *Store) EnqueueOffline(topic string, payload []byte) error {
	// 检查队列长度，超限则丢弃最旧的消息
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM offline_queue WHERE status = 'pending'`).Scan(&count); err != nil {
		return fmt.Errorf("failed to count offline queue: %w", err)
	}
	if count >= s.offlineQueueMax {
		if _, err := s.db.Exec(`DELETE FROM offline_queue WHERE id = (SELECT MIN(id) FROM offline_queue WHERE status = 'pending')`); err != nil {
			return fmt.Errorf("failed to evict oldest offline message: %w", err)
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO offline_queue (topic, payload, created_at, status) VALUES (?, ?, ?, 'pending')`,
		topic, payload, time.Now(),
	)
	return err
}

func (s *Store) DequeueOffline(limit int) ([]models.QueuedMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, topic, payload, created_at, retries, status FROM offline_queue
		 WHERE status = 'pending' ORDER BY created_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []models.QueuedMessage
	for rows.Next() {
		var m models.QueuedMessage
		if err := rows.Scan(&m.ID, &m.Topic, &m.Payload, &m.CreatedAt, &m.Retries, &m.Status); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (s *Store) MarkOfflineSent(id int64) error {
	_, err := s.db.Exec(`UPDATE offline_queue SET status = 'sent' WHERE id = ?`, id)
	return err
}

func (s *Store) IncrementOfflineRetry(id int64) error {
	_, err := s.db.Exec(`UPDATE offline_queue SET retries = retries + 1 WHERE id = ?`, id)
	return err
}

func (s *Store) MarkOfflineFailed(id int64) error {
	_, err := s.db.Exec(`UPDATE offline_queue SET status = 'failed' WHERE id = ?`, id)
	return err
}

func (s *Store) OfflineQueueLength() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM offline_queue WHERE status = 'pending'`).Scan(&count)
	return count, err
}

// ── 用户操作 ──

func (s *Store) SaveUser(user *models.User) error {
	_, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash, role = excluded.role`,
		user.Username, user.PasswordHash, user.Role, user.CreatedAt,
	)
	return err
}

// DeleteUser 删除用户
func (s *Store) DeleteUser(username string) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE username = ?`, username)
	return err
}

func (s *Store) GetUser(username string) (*models.User, error) {
	var u models.User
	var lastLogin sql.NullTime
	err := s.db.QueryRow(
		`SELECT username, password_hash, role, created_at, last_login FROM users WHERE username = ?`,
		username,
	).Scan(&u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &lastLogin)
	if err != nil {
		return nil, err
	}
	if lastLogin.Valid {
		u.LastLogin = &lastLogin.Time
	}
	return &u, nil
}

func (s *Store) UpdateLastLogin(username string) error {
	_, err := s.db.Exec(`UPDATE users SET last_login = ? WHERE username = ?`, time.Now(), username)
	return err
}

func (s *Store) ListUsers() ([]models.User, error) {
	rows, err := s.db.Query(`SELECT username, password_hash, role, created_at, last_login FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		var lastLogin sql.NullTime
		if err := rows.Scan(&u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &lastLogin); err != nil {
			return nil, err
		}
		if lastLogin.Valid {
			u.LastLogin = &lastLogin.Time
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// ── 模型操作 ──

func (s *Store) SaveModel(m *models.ModelInfo) error {
	_, err := s.db.Exec(
		`INSERT INTO models (id, name, version, type, file_path, partition, active, size_bytes, sha256, loaded_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, version=excluded.version, type=excluded.type,
		 file_path=excluded.file_path, partition=excluded.partition, active=excluded.active,
		 size_bytes=excluded.size_bytes, sha256=excluded.sha256, loaded_at=excluded.loaded_at`,
		m.ID, m.Name, m.Version, m.Type, m.FilePath, m.Partition, boolToInt(m.Active),
		m.SizeBytes, m.SHA256, m.LoadedAt,
	)
	return err
}

func (s *Store) ListModels() ([]models.ModelInfo, error) {
	rows, err := s.db.Query(
		`SELECT id, name, version, type, file_path, partition, active, size_bytes, sha256, loaded_at FROM models ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var modelsList []models.ModelInfo
	for rows.Next() {
		var m models.ModelInfo
		var active int
		var loadedAt sql.NullTime
		if err := rows.Scan(&m.ID, &m.Name, &m.Version, &m.Type, &m.FilePath, &m.Partition,
			&active, &m.SizeBytes, &m.SHA256, &loadedAt); err != nil {
			return nil, err
		}
		m.Active = active == 1
		if loadedAt.Valid {
			m.LoadedAt = &loadedAt.Time
		}
		modelsList = append(modelsList, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return modelsList, nil
}

func (s *Store) SetActiveModel(modelID string) error {
	// 直接激活指定模型，不影响其他模型的激活状态
	_, err := s.db.Exec(`UPDATE models SET active = 1 WHERE id = ?`, modelID)
	return err
}

// ToggleModel 切换模型激活状态（激活→取消，未激活→激活）
func (s *Store) ToggleModel(modelID string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var currentActive int
	err = tx.QueryRow(`SELECT active FROM models WHERE id = ?`, modelID).Scan(&currentActive)
	if err != nil {
		return false, fmt.Errorf("model not found: %s", modelID)
	}

	newActive := 1
	if currentActive == 1 {
		newActive = 0
	}

	if _, err := tx.Exec(`UPDATE models SET active = ? WHERE id = ?`, newActive, modelID); err != nil {
		return false, fmt.Errorf("failed to toggle model: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return newActive == 1, nil
}

// DeleteModel 从数据库中删除模型记录
func (s *Store) DeleteModel(modelID string) error {
	_, err := s.db.Exec(`DELETE FROM models WHERE id = ?`, modelID)
	return err
}

// ── 时序缓存 ──

func (s *Store) SaveTSData(deviceID, metricName string, value float64, timestamp int64, quality string) error {
	_, err := s.db.Exec(
		`INSERT INTO ts_cache (device_id, metric_name, value, timestamp, quality) VALUES (?, ?, ?, ?, ?)`,
		deviceID, metricName, value, timestamp, quality,
	)
	return err
}

func (s *Store) GetTSData(deviceID, metricName string, limit int) ([]map[string]any, error) {
	rows, err := s.db.Query(
		`SELECT device_id, metric_name, value, timestamp, quality FROM ts_cache
		 WHERE device_id = ? AND metric_name = ? ORDER BY timestamp DESC LIMIT ?`,
		deviceID, metricName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var devID, mName, quality string
		var value float64
		var ts int64
		if err := rows.Scan(&devID, &mName, &value, &ts, &quality); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"device_id":   devID,
			"metric_name": mName,
			"value":       value,
			"timestamp":   ts,
			"quality":     quality,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// ── 工作流操作 ──

func (s *Store) SaveWorkflow(name string, definition []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO workflows (name, definition, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET definition = excluded.definition, updated_at = excluded.updated_at`,
		name, string(definition), time.Now(),
	)
	return err
}

func (s *Store) ListWorkflows() ([]map[string]any, error) {
	rows, err := s.db.Query(`SELECT name, definition, created_at, updated_at FROM workflows ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var name, definition string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&name, &definition, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"name":       name,
			"definition": definition,
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Store) GetWorkflow(name string) ([]byte, error) {
	var definition string
	err := s.db.QueryRow(`SELECT definition FROM workflows WHERE name = ?`, name).Scan(&definition)
	if err != nil {
		return nil, err
	}
	return []byte(definition), nil
}

func (s *Store) DeleteWorkflow(name string) error {
	_, err := s.db.Exec(`DELETE FROM workflows WHERE name = ?`, name)
	return err
}

// ── OTA 任务 ──

func (s *Store) SaveOTATask(task *models.OTATask) error {
	_, err := s.db.Exec(
		`INSERT INTO ota_tasks (id, model_name, version, target_partition, status, sha256, size_bytes, created_at, completed_at, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET status=excluded.status, sha256=excluded.sha256,
		 size_bytes=excluded.size_bytes, completed_at=excluded.completed_at, error=excluded.error`,
		task.ID, task.ModelName, task.Version, task.TargetPartition, task.Status,
		task.SHA256, task.SizeBytes, task.CreatedAt, task.CompletedAt, task.Error,
	)
	return err
}

func (s *Store) ListOTATasks(limit int) ([]models.OTATask, error) {
	rows, err := s.db.Query(
		`SELECT id, model_name, version, target_partition, status, sha256, size_bytes, created_at, completed_at, error
		 FROM ota_tasks ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.OTATask
	for rows.Next() {
		var t models.OTATask
		var completedAt sql.NullTime
		var errStr sql.NullString
		if err := rows.Scan(&t.ID, &t.ModelName, &t.Version, &t.TargetPartition, &t.Status,
			&t.SHA256, &t.SizeBytes, &t.CreatedAt, &completedAt, &errStr); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}
		if errStr.Valid {
			t.Error = errStr.String
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

// ── 登录尝试 ──

func (s *Store) RecordLoginAttempt(username, ip string, success bool) error {
	_, err := s.db.Exec(
		`INSERT INTO login_attempts (username, ip, success, timestamp) VALUES (?, ?, ?, ?)`,
		username, ip, boolToInt(success), time.Now(),
	)
	return err
}

func (s *Store) GetFailedLoginCount(username string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM login_attempts WHERE username = ? AND success = 0 AND timestamp >= ?`,
		username, since,
	).Scan(&count)
	return count, err
}

// ── 数据清理 ──

func (s *Store) Cleanup() error {
	cutoff := time.Now().AddDate(0, 0, -s.retentionDays)
	auditCutoff := time.Now().AddDate(0, 0, -s.auditRetentionDays)

	if _, err := s.db.Exec(`DELETE FROM ts_cache WHERE timestamp < ?`, cutoff.UnixMilli()); err != nil {
		return fmt.Errorf("failed to cleanup ts_cache: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM audit_log WHERE timestamp < ?`, auditCutoff); err != nil {
		return fmt.Errorf("failed to cleanup audit_log: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM offline_queue WHERE status = 'sent' AND created_at < ?`, cutoff); err != nil {
		return fmt.Errorf("failed to cleanup offline_queue: %w", err)
	}
	return nil
}

// ── 辅助函数 ──

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
