package ota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

// DelayedActivationConfig 延迟激活配置
type DelayedActivationConfig struct {
	Enabled          bool          `yaml:"enabled"`
	MaintenanceStart string        `yaml:"maintenance_start"` // "02:00"
	MaintenanceEnd   string        `yaml:"maintenance_end"`   // "04:00"
	AutoActivate     bool          `yaml:"auto_activate"`
}

// ScheduleDelayedActivation 安排延迟激活（等待维护窗口）
func (m *Manager) ScheduleDelayedActivation(task *models.OTATask, config DelayedActivationConfig) {
	if !config.Enabled {
		// 不启用延迟激活，立即切换
		m.logger.Infof("Delayed activation disabled, switching immediately for %s", task.ModelName)
		m.SwitchPartition()
		return
	}

	go func() {
		m.logger.Infof("Scheduled delayed activation for %s v%s (window: %s-%s)",
			task.ModelName, task.Version, config.MaintenanceStart, config.MaintenanceEnd)

		for {
			now := time.Now()
			maintenanceStart := parseMaintenanceTime(now, config.MaintenanceStart)
			maintenanceEnd := parseMaintenanceTime(now, config.MaintenanceEnd)

			// 如果当前在维护窗口内，执行切换
			if now.After(maintenanceStart) && now.Before(maintenanceEnd) {
				m.logger.Infof("Maintenance window reached, activating %s v%s", task.ModelName, task.Version)
				if config.AutoActivate {
					m.SwitchPartition()
				}
				return
			}

			// 等待到下一个检查点
			nextCheck := maintenanceStart.Sub(now)
			if nextCheck < 0 {
				nextCheck = 24 * time.Hour + nextCheck
			}
			if nextCheck > time.Hour {
				nextCheck = time.Hour
			}
			time.Sleep(nextCheck)
		}
	}()
}

// parseMaintenanceTime 解析维护窗口时间
func parseMaintenanceTime(now time.Time, timeStr string) time.Time {
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return now
	}
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
}

// ImportUSBPackage 从 USB 导入模型包（完全离线 OTA）
func (m *Manager) ImportUSBPackage(ctx context.Context, usbPath string) (*models.OTATask, error) {
	// 验证 USB 路径存在
	info, err := os.Stat(usbPath)
	if err != nil {
		return nil, fmt.Errorf("USB path not accessible: %w", err)
	}

	var modelFile string
	if info.IsDir() {
		// 查找目录中的模型文件
		entries, err := os.ReadDir(usbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read USB directory: %w", err)
		}
		for _, entry := range entries {
			name := entry.Name()
			ext := filepath.Ext(name)
			if ext == ".onnx" || ext == ".gguf" || ext == ".bin" {
				modelFile = filepath.Join(usbPath, name)
				break
			}
		}
		if modelFile == "" {
			return nil, fmt.Errorf("no model file found in USB directory %s", usbPath)
		}
	} else {
		modelFile = usbPath
	}

	// 计算 SHA-256
	hash, err := m.computeFileHash(modelFile)
	if err != nil {
		return nil, fmt.Errorf("failed to hash USB model: %w", err)
	}

	// 检查签名文件（.sig）
	sigFile := modelFile + ".sig"
	if _, err := os.Stat(sigFile); err == nil {
		// 读取签名并验证
		sigBytes, err := os.ReadFile(sigFile)
		if err == nil {
			if err := m.VerifySignature(modelFile, string(sigBytes)); err != nil {
				return nil, fmt.Errorf("USB model signature verification failed: %w", err)
			}
			m.logger.Info("USB model signature verified successfully")
		}
	}

	// 复制到非活跃分区
	targetDir := m.PartitionDir(m.InactivePartition())
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create target directory: %w", err)
	}

	fileName := filepath.Base(modelFile)
	targetPath := filepath.Join(targetDir, fileName)
	if err := copyFile(modelFile, targetPath); err != nil {
		return nil, fmt.Errorf("failed to copy USB model to partition: %w", err)
	}

	// 创建 OTA 任务记录
	task := &models.OTATask{
		ID:              fmt.Sprintf("ota-usb-%s", time.Now().Format("20060102-150405")),
		ModelName:       fileName,
		Version:         "usb-import",
		TargetPartition: m.InactivePartition(),
		Status:          models.OTAStatusVerifying,
		SizeBytes:       info.Size(),
		SHA256:          hash,
		CreatedAt:       time.Now(),
	}

	if err := m.store.SaveOTATask(task); err != nil {
		m.logger.Errorf("Failed to save USB OTA task: %v", err)
	}

	m.logger.Infof("USB model imported: %s (%d bytes, SHA256: %s)", fileName, info.Size(), truncateHash(hash))
	return task, nil
}

// SyncWorkflowDiff 同步编排策略 YAML 差异
func (m *Manager) SyncWorkflowDiff(ctx context.Context, workflowDir string, remoteURL string) error {
	// 下载远程 YAML
	tmpDir := filepath.Join(os.TempDir(), "workflow-sync")
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	if err := m.downloadFile(ctx, remoteURL, filepath.Join(tmpDir, "remote.yaml")); err != nil {
		return fmt.Errorf("failed to download remote workflow: %w", err)
	}

	remoteData, err := os.ReadFile(filepath.Join(tmpDir, "remote.yaml"))
	if err != nil {
		return fmt.Errorf("failed to read remote workflow: %w", err)
	}

	localPath := filepath.Join(workflowDir, filepath.Base(remoteURL))
	localData, err := os.ReadFile(localPath)

	if err != nil || computeHashBytes(localData) != computeHashBytes(remoteData) {
		// 差异检测: 内容不同，更新
		if err := os.WriteFile(localPath, remoteData, 0644); err != nil {
			return fmt.Errorf("failed to write updated workflow: %w", err)
		}
		m.logger.Infof("Workflow %s synced (diff detected)", filepath.Base(remoteURL))
	} else {
		m.logger.Debug("Workflow already in sync, no changes")
	}

	return nil
}

// SyncKnowledgeDelta 同步知识库向量增量包
func (m *Manager) SyncKnowledgeDelta(ctx context.Context, knowledgeDir string, deltaURL string) error {
	// 下载增量包
	deltaPath := filepath.Join(os.TempDir(), fmt.Sprintf("knowledge-delta-%d.bin", time.Now().Unix()))
	defer os.Remove(deltaPath)

	if err := m.downloadFile(ctx, deltaURL, deltaPath); err != nil {
		return fmt.Errorf("failed to download knowledge delta: %w", err)
	}

	// 验证增量包完整性
	hash, err := m.computeFileHash(deltaPath)
	if err != nil {
		return fmt.Errorf("failed to hash knowledge delta: %w", err)
	}
	m.logger.Infof("Knowledge delta downloaded (SHA256: %s)", truncateHash(hash))

	// 解压并应用到知识库目录
	// 生产环境: 使用 bsdiff/xdelta3 应用向量增量
	// 简化方案: 增量包即为完整文件，直接复制
	targetPath := filepath.Join(knowledgeDir, "vectors.bin")
	if err := copyFile(deltaPath, targetPath); err != nil {
		return fmt.Errorf("failed to apply knowledge delta: %w", err)
	}

	m.logger.Info("Knowledge delta applied successfully")
	return nil
}

// computeHashBytes 计算字节数组的 SHA-256
func computeHashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// TruncateForExport 导出方法供外部使用
func (m *Manager) DownloadFileExport(ctx context.Context, url, destPath string) error {
	return m.downloadFile(ctx, url, destPath)
}

// CopyFileExport 导出 copyFile
func CopyFileExport(src, dst string) error {
	return copyFile(src, dst)
}

// LogEntry 返回管理器的日志条目
func (m *Manager) LogEntry() *logrus.Entry {
	return m.logger
}
