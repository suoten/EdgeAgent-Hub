package security

import (
	"testing"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
)

func TestPasswordHashing(t *testing.T) {
	password := "testPassword123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == password {
		t.Error("hash should not equal password")
	}

	if !CheckPassword(password, hash) {
		t.Error("CheckPassword should return true for correct password")
	}

	if CheckPassword("wrongPassword", hash) {
		t.Error("CheckPassword should return false for wrong password")
	}
}

func TestJWTGeneration(t *testing.T) {
	authSvc := NewAuthService("test-secret-key", "HS256", 30*time.Minute, 168*time.Hour, nil)

	token, err := authSvc.GenerateAccessToken("testuser", models.RoleAdmin)
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	if token == "" {
		t.Error("token should not be empty")
	}

	// Validate
	claims, err := authSvc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", claims.Username)
	}
	if claims.Role != models.RoleAdmin {
		t.Errorf("expected role admin, got %s", claims.Role)
	}
}

func TestJWTInvalidToken(t *testing.T) {
	authSvc := NewAuthService("test-secret-key", "HS256", 30*time.Minute, 168*time.Hour, nil)

	_, err := authSvc.ValidateToken("invalid.token.here")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestRBAC(t *testing.T) {
	// Admin should have all permissions
	if !HasPermission(models.RoleAdmin, PermModelLoad) {
		t.Error("admin should have model:load permission")
	}
	if !HasPermission(models.RoleAdmin, PermUserManage) {
		t.Error("admin should have user:manage permission")
	}

	// Viewer should have read-only permissions
	if !HasPermission(models.RoleViewer, PermModelList) {
		t.Error("viewer should have model:list permission")
	}
	if HasPermission(models.RoleViewer, PermModelLoad) {
		t.Error("viewer should NOT have model:load permission")
	}
	if HasPermission(models.RoleViewer, PermUserManage) {
		t.Error("viewer should NOT have user:manage permission")
	}

	// Operator should have most permissions except user management
	if !HasPermission(models.RoleOperator, PermModelLoad) {
		t.Error("operator should have model:load permission")
	}
	if HasPermission(models.RoleOperator, PermUserManage) {
		t.Error("operator should NOT have user:manage permission")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	// 前三次应该允许
	if !rl.Allow("192.168.1.1") {
		t.Error("first request should be allowed")
	}
	if !rl.Allow("192.168.1.1") {
		t.Error("second request should be allowed")
	}
	if !rl.Allow("192.168.1.1") {
		t.Error("third request should be allowed")
	}

	// 第四次应该拒绝
	if rl.Allow("192.168.1.1") {
		t.Error("fourth request should be rejected")
	}

	// 不同 IP 应该允许
	if !rl.Allow("192.168.1.2") {
		t.Error("different IP should be allowed")
	}
}

func TestLoginProtector(t *testing.T) {
	lp := NewLoginProtector(3, 5*time.Minute)

	// 前两次失败不应锁定
	lp.RecordFailure("user1")
	if lp.IsLocked("user1") {
		t.Error("user should not be locked after 1 failure")
	}

	lp.RecordFailure("user1")
	if lp.IsLocked("user1") {
		t.Error("user should not be locked after 2 failures")
	}

	// 第三次失败应该锁定
	lp.RecordFailure("user1")
	if !lp.IsLocked("user1") {
		t.Error("user should be locked after 3 failures")
	}

	// 成功应该重置
	lp.RecordSuccess("user1")
	if lp.IsLocked("user1") {
		t.Error("user should not be locked after success")
	}
}

func TestHMACSignature(t *testing.T) {
	secret := []byte("test-secret")
	data := []byte("test data")

	sig := ComputeHMAC(data, secret)
	if sig == "" {
		t.Error("HMAC should not be empty")
	}

	if !VerifyHMACSignature(data, sig, secret) {
		t.Error("HMAC verification should pass")
	}

	if VerifyHMACSignature(data, "wrong-signature", secret) {
		t.Error("HMAC verification should fail for wrong signature")
	}
}

func TestSHA256(t *testing.T) {
	data := []byte("test data")
	hash := ComputeSHA256(data)

	if len(hash) != 64 {
		t.Errorf("expected 64 char hex string, got %d", len(hash))
	}

	// 相同数据应该生成相同哈希
	hash2 := ComputeSHA256(data)
	if hash != hash2 {
		t.Error("SHA256 should be deterministic")
	}
}

func TestDesensitizer(t *testing.T) {
	d := NewDesensitizer([]string{"ip", "mac", "serial"}, nil)

	// IP 脱敏
	result := d.Desensitize("192.168.1.100")
	if result != "192.168.x.x" {
		t.Errorf("expected 192.168.x.x, got %s", result)
	}

	// MAC 脱敏
	result = d.Desensitize("AA:BB:CC:DD:EE:FF")
	if result != "AA:BB:CC:xx:xx:xx" {
		t.Errorf("expected AA:BB:CC:xx:xx:xx, got %s", result)
	}
}
