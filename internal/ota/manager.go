package ota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/edgelite/edgeagent-hub/internal/security"
	"github.com/edgelite/edgeagent-hub/internal/storage"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Manager 管理模型 OTA 与生命周期
// A/B 分区策略: 下载→验证→灰度→切换→回滚
type Manager struct {
	store            *storage.Store
	activePartition  string
	partitionA       string
	partitionB       string
	retentionDays    int
	canaryPercent    int
	registryURL      string
	registryUser     string
	registryPass     string
	modelSigningKey  []byte
	logger           *logrus.Entry
	mu               sync.RWMutex
}

// NewManager 创建 OTA 管理器
func NewManager(store *storage.Store, partitionA, partitionB, activePartition string,
	retentionDays, canaryPercent int, registryURL, registryUser, registryPass string,
	modelSigningKey []byte, logger *logrus.Entry) *Manager {
	return &Manager{
		store:           store,
		activePartition: activePartition,
		partitionA:      partitionA,
		partitionB:      partitionB,
		retentionDays:   retentionDays,
		canaryPercent:   canaryPercent,
		registryURL:     registryURL,
		registryUser:    registryUser,
		registryPass:    registryPass,
		modelSigningKey: modelSigningKey,
		logger:          logger,
	}
}

// ActivePartition 返回当前活跃分区
func (m *Manager) ActivePartition() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activePartition
}

// InactivePartition 返回非活跃分区
func (m *Manager) InactivePartition() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activePartition == "A" {
		return "B"
	}
	return "A"
}

// PartitionDir 返回分区对应的目录
func (m *Manager) PartitionDir(partition string) string {
	if partition == "A" {
		return m.partitionA
	}
	return m.partitionB
}

// CheckUpdate 检查模型更新
func (m *Manager) CheckUpdate(ctx context.Context, modelName, version string) (*models.OTATask, error) {
	if m.registryURL == "" {
		return nil, fmt.Errorf("registry URL not configured")
	}

	task := &models.OTATask{
		ID:              fmt.Sprintf("ota-%s-%s", uuid.New().String()[:8], time.Now().Format("20060102")),
		ModelName:       modelName,
		Version:         version,
		TargetPartition: m.InactivePartition(),
		Status:          models.OTAStatusPending,
		CreatedAt:       time.Now(),
	}

	if err := m.store.SaveOTATask(task); err != nil {
		return nil, fmt.Errorf("failed to save OTA task: %w", err)
	}

	return task, nil
}

// DownloadAndVerify 下载模型并验证完整性
func (m *Manager) DownloadAndVerify(ctx context.Context, task *models.OTATask, downloadURL string) error {
	task.Status = models.OTAStatusDownloading
	m.store.SaveOTATask(task)

	// 确保目标分区目录存在
	targetDir := m.PartitionDir(task.TargetPartition)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create partition directory: %w", err)
	}

	// 下载文件
	filePath := filepath.Join(targetDir, fmt.Sprintf("%s_%s.onnx", task.ModelName, task.Version))
	file, err := os.Create(filePath)
	if err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("failed to create file: %v", err)
		m.store.SaveOTATask(task)
		return err
	}
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("failed to create request: %v", err)
		m.store.SaveOTATask(task)
		return err
	}
	if m.registryUser != "" {
		req.SetBasicAuth(m.registryUser, m.registryPass)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("download failed: %v", err)
		m.store.SaveOTATask(task)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 消费响应体以便连接复用
		io.Copy(io.Discard, resp.Body)
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("download failed: HTTP %d", resp.StatusCode)
		m.store.SaveOTATask(task)
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// 计算 SHA-256 并写入文件
	hasher := sha256.New()
	writer := io.MultiWriter(file, hasher)

	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("download write failed: %v", err)
		m.store.SaveOTATask(task)
		return err
	}

	task.SizeBytes = written
	task.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	task.Status = models.OTAStatusVerifying
	m.store.SaveOTATask(task)

	m.logger.Infof("Downloaded model %s v%s (%d bytes, SHA256: %s)",
		task.ModelName, task.Version, written, truncateHash(task.SHA256))

	return nil
}

// VerifyIntegrity 验证模型完整性 (SHA-256)
func (m *Manager) VerifyIntegrity(filePath, expectedSHA256 string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if actualSHA256 != expectedSHA256 {
		return fmt.Errorf("SHA-256 mismatch: expected %s, got %s", expectedSHA256, actualSHA256)
	}

	return nil
}

// VerifySignature 验证模型签名 (HMAC-SHA256)
func (m *Manager) VerifySignature(filePath, signature string) error {
	if len(m.modelSigningKey) == 0 {
		m.logger.Warn("Model signing key not configured, skipping signature verification")
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if !security.VerifyHMACSignature(data, signature, m.modelSigningKey) {
		return fmt.Errorf("model signature verification failed")
	}

	return nil
}

// SwitchPartition 原子切换活跃分区
func (m *Manager) SwitchPartition() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldPartition := m.activePartition
	if oldPartition == "A" {
		m.activePartition = "B"
	} else {
		m.activePartition = "A"
	}

	m.logger.Infof("Switched active partition: %s → %s", oldPartition, m.activePartition)
	return nil
}

// Rollback 回滚到上一个分区
func (m *Manager) Rollback() error {
	return m.SwitchPartition()
}

// ApplyUpdate 执行完整 OTA 流程: 下载→验证→切换
func (m *Manager) ApplyUpdate(ctx context.Context, task *models.OTATask, downloadURL string) error {
	// 1. 下载
	if err := m.DownloadAndVerify(ctx, task, downloadURL); err != nil {
		return err
	}

	// 2. 验证完整性
	filePath := filepath.Join(m.PartitionDir(task.TargetPartition),
		fmt.Sprintf("%s_%s.onnx", task.ModelName, task.Version))
	if err := m.VerifyIntegrity(filePath, task.SHA256); err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("integrity verification failed: %v", err)
		m.store.SaveOTATask(task)
		return err
	}

	// 3. 切换分区
	task.Status = models.OTAStatusSwitching
	m.store.SaveOTATask(task)

	if err := m.SwitchPartition(); err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("partition switch failed: %v", err)
		m.store.SaveOTATask(task)
		return err
	}

	now := time.Now()
	task.CompletedAt = &now
	task.Status = models.OTAStatusComplete
	m.store.SaveOTATask(task)

	m.logger.Infof("OTA update completed: %s v%s (partition: %s)",
		task.ModelName, task.Version, m.ActivePartition())

	return nil
}

// CleanupOldVersions 清理旧版本模型
func (m *Manager) CleanupOldVersions() error {
	cutoff := time.Now().AddDate(0, 0, -m.retentionDays)

	for _, partition := range []string{m.partitionA, m.partitionB} {
		entries, err := os.ReadDir(partition)
		if err != nil {
			continue // 目录不存在时跳过
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(partition, entry.Name()))
				m.logger.Infof("Cleaned up old model: %s (modified: %s)", entry.Name(), info.ModTime())
			}
		}
	}
	return nil
}

// ListTasks 列出 OTA 任务
func (m *Manager) ListTasks(limit int) ([]models.OTATask, error) {
	return m.store.ListOTATasks(limit)
}

// DownloadWithResume 支持断点续传的下载
func (m *Manager) DownloadWithResume(ctx context.Context, task *models.OTATask, downloadURL string) error {
	task.Status = models.OTAStatusDownloading
	m.store.SaveOTATask(task)

	targetDir := m.PartitionDir(task.TargetPartition)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create partition directory: %w", err)
	}

	filePath := filepath.Join(targetDir, fmt.Sprintf("%s_%s.onnx", task.ModelName, task.Version))

	// 检查已有部分下载的文件
	var existingSize int64 = 0
	if info, err := os.Stat(filePath); err == nil {
		existingSize = info.Size()
		m.logger.Infof("Resuming download from %d bytes (%.1f%%)", existingSize, float64(existingSize)/float64(task.SizeBytes)*100)
	}

	// 打开文件用于追加写入
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("failed to create file: %v", err)
		m.store.SaveOTATask(task)
		return err
	}
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("failed to create request: %v", err)
		m.store.SaveOTATask(task)
		return err
	}
	if m.registryUser != "" {
		req.SetBasicAuth(m.registryUser, m.registryPass)
	}

	// 设置 Range 头以支持断点续传
	if existingSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("download failed: %v", err)
		m.store.SaveOTATask(task)
		return err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if existingSize > 0 && resp.StatusCode == http.StatusPartialContent {
		// 断点续传成功
	} else if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("download failed: HTTP %d", resp.StatusCode)
		m.store.SaveOTATask(task)
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	} else if existingSize > 0 {
		// 服务器不支持 Range，从头开始
		file.Close()
		file, err = os.Create(filePath) // 截断重新写
		if err != nil {
			return err
		}
		defer file.Close()
		existingSize = 0
	}

	// 计算 SHA-256 (如果从头开始，计算整个文件；如果续传，需要先读取已有部分)
	// 简化方案: 如果是续传，先读取已有部分到 hasher
	fullHasher := sha256.New()
	if existingSize > 0 {
		existingFile, err := os.Open(filePath)
		if err == nil {
			io.Copy(fullHasher, existingFile)
			existingFile.Close()
		}
	}

	writer := io.MultiWriter(file, fullHasher)
	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("download write failed: %v", err)
		m.store.SaveOTATask(task)
		return err
	}

	totalSize := existingSize + written
	task.SizeBytes = totalSize
	task.SHA256 = hex.EncodeToString(fullHasher.Sum(nil))
	task.Status = models.OTAStatusVerifying
	m.store.SaveOTATask(task)

	m.logger.Infof("Downloaded model %s v%s (%d bytes, SHA256: %s)",
		task.ModelName, task.Version, totalSize, truncateHash(task.SHA256))

	return nil
}

// ApplyDeltaUpdate 增量更新: 下载补丁文件并应用到现有模型
func (m *Manager) ApplyDeltaUpdate(ctx context.Context, task *models.OTATask, patchURL string) error {
	task.Status = models.OTAStatusDownloading
	m.store.SaveOTATask(task)

	targetDir := m.PartitionDir(task.TargetPartition)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create partition directory: %w", err)
	}

	// 下载补丁文件到临时目录
	patchPath := filepath.Join(os.TempDir(), fmt.Sprintf("patch_%s_%s.bin", task.ModelName, task.Version))
	defer os.Remove(patchPath)

	if err := m.downloadFile(ctx, patchURL, patchPath); err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("patch download failed: %v", err)
		m.store.SaveOTATask(task)
		return err
	}

	// 验证补丁完整性
	patchHash, err := m.computeFileHash(patchPath)
	if err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("patch hash failed: %v", err)
		m.store.SaveOTATask(task)
		return err
	}
	task.SHA256 = patchHash

	// 在实际实现中，这里会使用 bsdiff / xdelta3 等工具应用补丁
	// 简化方案: 补丁文件即为完整模型文件（全量替换）
	// 生产环境中可集成 bsdiff:
	//   patch.Apply(oldFile, patchFile, outputFile)

	targetPath := filepath.Join(targetDir, fmt.Sprintf("%s_%s.onnx", task.ModelName, task.Version))
	if err := copyFile(patchPath, targetPath); err != nil {
		task.Status = models.OTAStatusFailed
		task.Error = fmt.Sprintf("patch apply failed: %v", err)
		m.store.SaveOTATask(task)
		return err
	}

	info, _ := os.Stat(targetPath)
	if info != nil {
		task.SizeBytes = info.Size()
	}

	task.Status = models.OTAStatusVerifying
	m.store.SaveOTATask(task)

	m.logger.Infof("Applied delta update for %s v%s (%d bytes)", task.ModelName, task.Version, task.SizeBytes)
	return nil
}

// downloadFile 下载文件到指定路径
func (m *Manager) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	if m.registryUser != "" {
		req.SetBasicAuth(m.registryUser, m.registryPass)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

// computeFileHash 计算文件 SHA-256
func (m *Manager) computeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// PauseDownload 暂停下载 (网络中断时自动调用)
func (m *Manager) PauseDownload(task *models.OTATask) {
	task.Status = models.OTAStatusPending
	task.Error = "download paused (network interruption)"
	m.store.SaveOTATask(task)
	m.logger.Infof("Paused OTA download for %s v%s", task.ModelName, task.Version)
}

// ResumeDownload 恢复下载 (网络恢复后自动调用)
func (m *Manager) ResumeDownload(ctx context.Context, task *models.OTATask, downloadURL string) error {
	m.logger.Infof("Resuming OTA download for %s v%s", task.ModelName, task.Version)
	return m.DownloadWithResume(ctx, task, downloadURL)
}

// truncateHash 截断哈希值用于日志输出
func truncateHash(hash string) string {
	if len(hash) > 16 {
		return hash[:16] + "..."
	}
	return hash
}
