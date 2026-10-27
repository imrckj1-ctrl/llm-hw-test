// Package auth 单密码登录:校验、会话、锁定(§5.1 / §7.8)
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"llmhwtest/internal/store"
)

const (
	sessionCookie = "hwtest_session"
	sessionTTL    = 7 * 24 * time.Hour // 登录态保持 7 天(F-1.2)
	maxFails      = 5                  // 连续输错 5 次(F-1.4)
	lockDuration  = 10 * time.Minute   // 锁定 10 分钟
)

type Auth struct {
	st *store.Store

	mu         sync.Mutex
	failCount  int
	lockedTill time.Time
}

func New(st *store.Store) *Auth { return &Auth{st: st} }

// IsFirstRun 尚未设置密码哈希
func (a *Auth) IsFirstRun() bool {
	_, ok := a.st.GetSetting("password_hash")
	return !ok
}

// ForceChange 首次登录须强制修改密码(F-1.5)
func (a *Auth) ForceChange() bool {
	v, _ := a.st.GetSetting("force_password_change")
	return v == "1"
}

// ResetPassword 生成 12 位随机初始密码
func (a *Auth) ResetPassword() (string, error) {
	pw := randomPassword(12)
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if err := a.st.SetSetting("password_hash", string(h)); err != nil {
		return "", err
	}
	if err := a.st.SetSetting("force_password_change", "1"); err != nil {
		return "", err
	}
	a.st.Exec(`DELETE FROM sessions`)
	return pw, nil
}

func randomPassword(n int) string {
	const letters = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

// Login 校验密码。返回 (会话token, 锁定剩余, 错误)
func (a *Auth) Login(password string) (string, time.Duration, error) {
	a.mu.Lock()
	if time.Now().Before(a.lockedTill) {
		remain := time.Until(a.lockedTill)
		a.mu.Unlock()
		return "", remain, fmt.Errorf("连续输错次数过多,已锁定")
	}
	a.mu.Unlock()

	hash, ok := a.st.GetSetting("password_hash")
	if !ok || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		a.mu.Lock()
		a.failCount++
		fc := a.failCount
		if a.failCount >= maxFails {
			a.lockedTill = time.Now().Add(lockDuration)
			a.failCount = 0
			fc = maxFails
		}
		a.mu.Unlock()
		return "", 0, fmt.Errorf("密码错误(第 %d/%d 次)", fc, maxFails)
	}
	a.mu.Lock()
	a.failCount = 0
	a.mu.Unlock()

	raw := make([]byte, 32)
	rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	now := time.Now()
	_, err := a.st.Exec(`INSERT INTO sessions(token_hash,created_at,expires_at) VALUES(?,?,?)`,
		hex.EncodeToString(sum[:]), now.UnixMilli(), now.Add(sessionTTL).UnixMilli())
	if err != nil {
		return "", 0, err
	}
	a.st.Exec(`DELETE FROM sessions WHERE expires_at<?`, now.UnixMilli())
	return token, 0, nil
}

func (a *Auth) Logout(token string) {
	sum := sha256.Sum256([]byte(token))
	a.st.Exec(`DELETE FROM sessions WHERE token_hash=?`, hex.EncodeToString(sum[:]))
}

func (a *Auth) ValidToken(token string) bool {
	if token == "" {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	var exp int64
	err := a.st.QueryRow(`SELECT expires_at FROM sessions WHERE token_hash=?`, hex.EncodeToString(sum[:])).Scan(&exp)
	return err == nil && exp > time.Now().UnixMilli()
}

// ---------- 长期 API Token(智能体/CI 用;明文仅生成时返回一次,库中只存 sha256) ----------

// GenerateAPIToken 生成新 token(覆盖旧的);返回明文,仅此一次可见
func (a *Auth) GenerateAPIToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := "hwt_" + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	if err := a.st.SetSetting("api_token_hash", hex.EncodeToString(sum[:])); err != nil {
		return "", err
	}
	a.st.SetSetting("api_token_created", fmt.Sprintf("%d", time.Now().UnixMilli()))
	return token, nil
}

// RevokeAPIToken 吊销(立即生效)
func (a *Auth) RevokeAPIToken() error {
	if err := a.st.SetSetting("api_token_hash", ""); err != nil {
		return err
	}
	return a.st.SetSetting("api_token_created", "")
}

// APITokenInfo 设置页展示用(不含明文)
func (a *Auth) APITokenInfo() (set bool, createdAt int64) {
	h, ok := a.st.GetSetting("api_token_hash")
	if !ok || h == "" {
		return false, 0
	}
	var ts int64
	if v, ok := a.st.GetSetting("api_token_created"); ok {
		fmt.Sscanf(v, "%d", &ts)
	}
	return true, ts
}

// ValidAPIToken 校验长期 API Token
func (a *Auth) ValidAPIToken(token string) bool {
	h, ok := a.st.GetSetting("api_token_hash")
	if !ok || h == "" || token == "" {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:]) == h
}

// bearerFrom 提取 Authorization: Bearer <token>
func bearerFrom(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

// ChangePassword 修改密码;已登录会话保持有效(单用户平台,改密不踢人)
func (a *Auth) ChangePassword(oldPw, newPw string) error {
	hash, _ := a.st.GetSetting("password_hash")
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(oldPw)) != nil {
		return fmt.Errorf("原密码错误")
	}
	if len(newPw) < 8 {
		return fmt.Errorf("新密码至少 8 位")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(newPw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := a.st.SetSetting("password_hash", string(h)); err != nil {
		return err
	}
	a.st.SetSetting("force_password_change", "0")
	return nil
}

// SetCookie / ClearCookie / TokenFrom
func SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
}

func TokenFrom(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// Middleware 保护 /api
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.ValidToken(TokenFrom(r)) && !a.ValidAPIToken(bearerFrom(r)) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"未登录或会话已过期"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
