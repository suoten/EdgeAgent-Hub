package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

// AuthService 处理 JWT 认证和 RBAC 授权
type AuthService struct {
	secret         []byte
	algorithm      string
	accessTTL      time.Duration
	refreshTTL     time.Duration
	logger         *logrus.Entry
}

// Claims 是 JWT 自定义声明
type Claims struct {
	Username string      `json:"username"`
	Role     models.Role `json:"role"`
	jwt.RegisteredClaims
}

// NewAuthService 创建认证服务
func NewAuthService(secret, algorithm string, accessTTL, refreshTTL time.Duration, logger *logrus.Entry) *AuthService {
	return &AuthService{
		secret:     []byte(secret),
		algorithm:  algorithm,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		logger:     logger,
	}
}

// GenerateAccessToken 生成 Access Token
func (a *AuthService) GenerateAccessToken(username string, role models.Role) (string, error) {
	claims := &Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   username,
			Issuer:    "edgeagent-hub",
		},
	}

	token := jwt.NewWithClaims(a.signingMethod(), claims)
	return token.SignedString(a.secret)
}

// GenerateRefreshToken 生成 Refresh Token
func (a *AuthService) GenerateRefreshToken(username string, role models.Role) (string, error) {
	claims := &Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.refreshTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   username,
			Issuer:    "edgeagent-hub",
		},
	}

	token := jwt.NewWithClaims(a.signingMethod(), claims)
	return token.SignedString(a.secret)
}

// ValidateToken 验证 Token 并返回声明
func (a *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != a.algorithm {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Header["alg"])
		}
		return a.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// HashPassword 使用 bcrypt 哈希密码
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword 验证密码
func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// signingMethod 返回 JWT 签名方法
func (a *AuthService) signingMethod() jwt.SigningMethod {
	switch a.algorithm {
	case "HS256":
		return jwt.SigningMethodHS256
	case "HS384":
		return jwt.SigningMethodHS384
	case "HS512":
		return jwt.SigningMethodHS512
	default:
		return jwt.SigningMethodHS256
	}
}

// ── RBAC 权限矩阵 ──

// Permission 定义权限
type Permission string

const (
	PermModelLoad    Permission = "model:load"
	PermModelUnload  Permission = "model:unload"
	PermModelList    Permission = "model:list"
	PermModelSwitch  Permission = "model:switch"
	PermModelRollback Permission = "model:rollback"
	PermWorkflowList   Permission = "workflow:list"
	PermWorkflowCreate Permission = "workflow:create"
	PermWorkflowUpdate Permission = "workflow:update"
	PermWorkflowTrigger Permission = "workflow:trigger"
	PermDeviceList    Permission = "device:list"
	PermAlertList     Permission = "alert:list"
	PermAlertAck      Permission = "alert:ack"
	PermKnowledgeUpload Permission = "knowledge:upload"
	PermKnowledgeSearch Permission = "knowledge:search"
	PermOTACheck      Permission = "ota:check"
	PermOTAApply      Permission = "ota:apply"
	PermSystemHealth  Permission = "system:health"
	PermSystemMetrics Permission = "system:metrics"
	PermChat          Permission = "chat"
	PermAuditLogs     Permission = "audit:logs"
	PermUserManage    Permission = "user:manage"
)

// rolePermissions 定义角色权限映射 (3 角色 x 22 权限)
var rolePermissions = map[models.Role][]Permission{
	models.RoleAdmin: {
		PermModelLoad, PermModelUnload, PermModelList, PermModelSwitch, PermModelRollback,
		PermWorkflowList, PermWorkflowCreate, PermWorkflowUpdate, PermWorkflowTrigger,
		PermDeviceList, PermAlertList, PermAlertAck,
		PermKnowledgeUpload, PermKnowledgeSearch,
		PermOTACheck, PermOTAApply,
		PermSystemHealth, PermSystemMetrics, PermChat,
		PermAuditLogs, PermUserManage,
	},
	models.RoleOperator: {
		PermModelLoad, PermModelUnload, PermModelList, PermModelSwitch,
		PermWorkflowList, PermWorkflowCreate, PermWorkflowUpdate, PermWorkflowTrigger,
		PermDeviceList, PermAlertList, PermAlertAck,
		PermKnowledgeUpload, PermKnowledgeSearch,
		PermOTACheck,
		PermSystemHealth, PermSystemMetrics, PermChat,
	},
	models.RoleViewer: {
		PermModelList, PermWorkflowList, PermDeviceList, PermAlertList,
		PermKnowledgeSearch,
		PermSystemHealth, PermSystemMetrics, PermChat,
	},
}

// HasPermission 检查角色是否拥有权限
func HasPermission(role models.Role, perm Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// ── HMAC 签名验证 (用于模型签名) ──

// VerifyHMACSignature 验证 HMAC-SHA256 签名
func VerifyHMACSignature(data []byte, signature string, secret []byte) bool {
	expected := ComputeHMAC(data, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ComputeHMAC 计算 HMAC-SHA256
func ComputeHMAC(data []byte, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeSHA256 计算 SHA-256
func ComputeSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ── 数据脱敏 ──

// Desensitizer 数据脱敏器
type Desensitizer struct {
	patterns []*patternReplacer
	logger   *logrus.Entry
}

type patternReplacer struct {
	re      *strings.Replacer
	pattern string
}

// NewDesensitizer 创建脱敏器
// patterns 中匹配项不区分大小写: ip, mac, serial
func NewDesensitizer(patterns []string, logger *logrus.Entry) *Desensitizer {
	d := &Desensitizer{logger: logger}
	for _, p := range patterns {
		lower := strings.ToLower(p)
		switch {
		case strings.Contains(lower, "ip"):
			d.patterns = append(d.patterns, &patternReplacer{pattern: "ip"})
		case strings.Contains(lower, "mac"):
			d.patterns = append(d.patterns, &patternReplacer{pattern: "mac"})
		case strings.Contains(lower, "serial") || strings.Contains(p, "序列号"):
			d.patterns = append(d.patterns, &patternReplacer{pattern: "serial"})
		}
	}
	return d
}

// Desensitize 对字符串进行脱敏
func (d *Desensitizer) Desensitize(s string) string {
	for _, p := range d.patterns {
		switch p.pattern {
		case "ip":
			s = desensitizeIP(s)
		case "mac":
			s = desensitizeMAC(s)
		case "serial":
			s = desensitizeSerial(s)
		}
	}
	return s
}

// desensitizeIP 脱敏 IP 地址 (192.168.1.100 → 192.168.x.x)
func desensitizeIP(s string) string {
	// 简化实现: 替换最后两段
	parts := strings.Split(s, ".")
	if len(parts) == 4 {
		// 检查是否像 IP
		if isNumeric(parts[0]) && isNumeric(parts[1]) && isNumeric(parts[2]) && isNumeric(parts[3]) {
			return parts[0] + "." + parts[1] + ".x.x"
		}
	}
	return s
}

// desensitizeMAC 脱敏 MAC 地址 (AA:BB:CC:DD:EE:FF → AA:BB:CC:xx:xx:xx)
func desensitizeMAC(s string) string {
	parts := strings.Split(s, ":")
	if len(parts) == 6 {
		return parts[0] + ":" + parts[1] + ":" + parts[2] + ":xx:xx:xx"
	}
	parts = strings.Split(s, "-")
	if len(parts) == 6 {
		return parts[0] + "-" + parts[1] + "-" + parts[2] + "-xx-xx-xx"
	}
	return s
}

// desensitizeSerial 脱敏序列号 (ABC123DEF456 → ABC1xxxxxF456)
func desensitizeSerial(s string) string {
	// 跳过 IP 地址和 MAC 地址格式
	if strings.Contains(s, ".") || strings.Contains(s, ":") || strings.Contains(s, "-") {
		return s
	}
	if len(s) > 10 && len(s) <= 17 {
		// 保留前4后4
		mid := len(s) - 8
		return s[:4] + strings.Repeat("x", mid) + s[len(s)-4:]
	}
	return s
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ── HMAC-SHA512 用于 HS512 JWT ──

// computeHMACSHA512 计算 HMAC-SHA512
func computeHMACSHA512(data []byte, secret []byte) string {
	var h hash.Hash
	h = hmac.New(sha512.New, secret)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
