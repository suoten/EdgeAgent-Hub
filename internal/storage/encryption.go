package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// SQLCipherConfig SQLCipher 加密配置
type SQLCipherConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Key      string `yaml:"key" json:"-"` // 加密密钥，不序列化
	Cipher   string `yaml:"cipher" json:"cipher"` // aes-256-cbc (默认)
}

// OpenWithEncryption 使用 SQLCipher 加密打开数据库
// 注意: modernc.org/sqlite 纯 Go 实现不支持 SQLCipher
// 生产环境需要:
//   1. 使用 mattn/go-sqlite3 + CGO 编译，配合 SQLCipher
//   2. 或在文件系统层使用 fscrypt/luks 加密
// 此函数提供框架接口和回退方案
func OpenWithEncryption(dbPath string, config SQLCipherConfig) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	// 构建 DSN
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", dbPath)

	// 如果启用加密，尝试设置 PRAGMA key
	// modernc.org/sqlite 不支持 SQLCipher 的 PRAGMA key
	// 但我们仍然打开数据库，加密由文件系统层处理
	if config.Enabled {
		// 生产环境 (使用 go-sqlite3 + sqlcipher):
		// dsn = fmt.Sprintf("file:%s?_pragma=key(%s)&_pragma=cipher_page_size(4096)", dbPath, config.Key)
		// 此处记录警告，建议使用文件系统加密
		fmt.Fprintf(os.Stderr, "WARNING: SQLCipher not supported by pure-Go sqlite driver. Use filesystem-level encryption (LUKS/fscrypt) for data at rest.\n")
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	return db, nil
}
