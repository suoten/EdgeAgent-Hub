package storage

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Backup 创建系统备份（配置 + 知识库 + 审计日志）
func (s *Store) Backup(outputPath string) error {
	zipFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 1. 备份数据库文件
	if err := addFileToZip(zipWriter, s.dbPath, "data/edgeagent.db"); err != nil {
		return fmt.Errorf("failed to backup database: %w", err)
	}

	// 2. 备份 WAL 文件（如果存在）
	walPath := s.dbPath + "-wal"
	if _, err := os.Stat(walPath); err == nil {
		if err := addFileToZip(zipWriter, walPath, "data/edgeagent.db-wal"); err != nil {
			return fmt.Errorf("failed to backup WAL: %w", err)
		}
	}

	// 3. 备份配置文件
	configPath := "configs/config.yaml"
	if _, err := os.Stat(configPath); err == nil {
		if err := addFileToZip(zipWriter, configPath, "configs/config.yaml"); err != nil {
			return fmt.Errorf("failed to backup config: %w", err)
		}
	}

	// 4. 备份工作流
	workflowDir := "workflows"
	if entries, err := os.ReadDir(workflowDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				path := filepath.Join(workflowDir, entry.Name())
				zipPath := filepath.Join("workflows", entry.Name())
				addFileToZip(zipWriter, path, zipPath)
			}
		}
	}

	// 5. 备份知识库
	knowledgeDir := "data/knowledge"
	if entries, err := os.ReadDir(knowledgeDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				path := filepath.Join(knowledgeDir, entry.Name())
				zipPath := filepath.Join("knowledge", entry.Name())
				addFileToZip(zipWriter, path, zipPath)
			}
		}
	}

	// 6. 写入备份元信息
	metaPath := filepath.Join(os.TempDir(), "backup_meta.txt")
	os.WriteFile(metaPath, []byte(fmt.Sprintf("backup_time: %s\n", time.Now().Format(time.RFC3339))), 0644)
	addFileToZip(zipWriter, metaPath, "backup_meta.txt")
	os.Remove(metaPath)

	return nil
}

// Restore 从备份恢复系统
func (s *Store) Restore(backupPath, targetDir string) error {
	zipReader, err := zip.OpenReader(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer zipReader.Close()

	for _, file := range zipReader.File {
		targetPath := filepath.Join(targetDir, file.Name)

		// 防止路径穿越
		if !isPathSafe(targetPath, targetDir) {
			continue
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(targetPath), 0755)

		dst, err := os.Create(targetPath)
		if err != nil {
			continue
		}

		src, err := file.Open()
		if err != nil {
			dst.Close()
			continue
		}

		io.Copy(dst, src)
		dst.Close()
		src.Close()
	}

	return nil
}

// addFileToZip 将文件添加到 ZIP
func addFileToZip(zipWriter *zip.Writer, sourcePath, zipPath string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = zipPath
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

// isPathSafe 检查路径是否安全（防止路径穿越）
func isPathSafe(targetPath, baseDir string) bool {
	rel, err := filepath.Rel(baseDir, targetPath)
	if err != nil {
		return false
	}
	if rel == ".." || len(rel) >= 2 && rel[:2] == ".." {
		return false
	}
	return true
}
