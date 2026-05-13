package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	oauthStateTTL  = 10 * time.Minute
	oauthTicketTTL = 2 * time.Minute
)

type oauthTicket struct {
	tokens tokenPair
	exp    time.Time
}

type oauthTicketStore struct {
	mu      sync.Mutex
	tickets map[string]oauthTicket
}

func newOAuthTicketStore() *oauthTicketStore {
	return &oauthTicketStore{tickets: map[string]oauthTicket{}}
}

func (s *oauthTicketStore) put(tokens tokenPair, ttl time.Duration) string {
	ticket := randomToken()
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, t := range s.tickets {
		if now.After(t.exp) {
			delete(s.tickets, k)
		}
	}
	s.tickets[ticket] = oauthTicket{tokens: tokens, exp: now.Add(ttl)}
	return ticket
}

func (s *oauthTicketStore) consume(ticket string) (tokenPair, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.tickets[ticket]
	if !ok {
		return tokenPair{}, false
	}
	delete(s.tickets, ticket)
	return entry.tokens, time.Now().Before(entry.exp)
}

// issueZhihuOAuthState 生成自校验 state，不依赖进程内存或 Cookie，便于多副本 / 滚动发布。
func (a *App) issueZhihuOAuthState() (string, error) {
	if strings.TrimSpace(a.cfg.JWTSecret) == "" {
		return "", errors.New("JWT_SECRET 未配置")
	}
	exp := time.Now().Add(oauthStateTTL).Unix()
	payload, err := json.Marshal(map[string]any{"exp": exp, "n": randomToken()})
	if err != nil {
		return "", err
	}
	part := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(a.cfg.JWTSecret))
	mac.Write([]byte(part))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return part + "." + sig, nil
}

func (a *App) verifyZhihuOAuthState(state string) bool {
	if state == "" || strings.TrimSpace(a.cfg.JWTSecret) == "" {
		return false
	}
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return false
	}
	part, sigB64 := parts[0], parts[1]
	mac := hmac.New(sha256.New, []byte(a.cfg.JWTSecret))
	mac.Write([]byte(part))
	want := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil || subtle.ConstantTimeCompare(want, got) != 1 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return false
	}
	var p struct {
		Exp int64  `json:"exp"`
		N   string `json:"n"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.N == "" {
		return false
	}
	return p.Exp >= time.Now().Unix()
}

func (a *App) zhihuOAuthStart(c *gin.Context) {
	if a.cfg.ZhihuAppID == "" || a.cfg.ZhihuAppKey == "" {
		fail(c, http.StatusServiceUnavailable, "知乎登录未启用，请配置 ZHIHU_APP_ID 和 ZHIHU_APP_KEY")
		return
	}

	state, err := a.issueZhihuOAuthState()
	if err != nil {
		fail(c, http.StatusInternalServerError, "生成登录校验参数失败")
		return
	}

	values := url.Values{}
	values.Set("redirect_uri", a.cfg.ZhihuRedirectURI)
	values.Set("app_id", a.cfg.ZhihuAppID)
	values.Set("response_type", "code")
	values.Set("state", state)
	ok(c, gin.H{"authorizeUrl": fmt.Sprintf("https://%s/authorize?%s", normalizeHost(a.cfg.ZhihuOpenAPIHost), values.Encode())})
}

func (a *App) zhihuOAuthCallback(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		code = strings.TrimSpace(c.Query("authorization_code"))
	}
	state := strings.TrimSpace(c.Query("state"))
	if code == "" {
		a.redirectOAuthError(c, "缺少知乎授权码")
		return
	}
	// 知乎部分授权回跳只带 authorization_code，不回传 state（与 OAuth2 理想行为不一致）；有 state 时再校验 CSRF。
	if state != "" && !a.verifyZhihuOAuthState(state) {
		a.redirectOAuthError(c, "知乎登录状态校验失败，请重新发起登录")
		return
	}

	accessToken, err := a.exchangeZhihuAccessToken(c.Request.Context(), code)
	if err != nil {
		a.redirectOAuthError(c, err.Error())
		return
	}
	zhihuUser, err := a.fetchZhihuUser(c.Request.Context(), accessToken)
	if err != nil {
		a.redirectOAuthError(c, err.Error())
		return
	}
	user, err := a.upsertUserFromZhihu(zhihuUser)
	if err != nil {
		a.redirectOAuthError(c, err.Error())
		return
	}
	tokens, err := a.issueTokens(user)
	if err != nil {
		a.redirectOAuthError(c, "签发登录态失败")
		return
	}

	ticket := a.oauthTickets.put(tokens, oauthTicketTTL)
	redirectURL, err := url.Parse(strings.TrimRight(a.cfg.FrontendOrigin, "/") + "/oauth/zhihu/callback")
	if err != nil {
		fail(c, http.StatusInternalServerError, "前端回跳地址配置无效")
		return
	}
	query := redirectURL.Query()
	query.Set("ticket", ticket)
	redirectURL.RawQuery = query.Encode()
	c.Redirect(http.StatusFound, redirectURL.String())
}

func (a *App) zhihuOAuthExchange(c *gin.Context) {
	var req struct {
		Ticket string `json:"ticket"`
	}
	if !bind(c, &req) {
		return
	}
	tokens, valid := a.oauthTickets.consume(strings.TrimSpace(req.Ticket))
	if !valid {
		fail(c, http.StatusUnauthorized, "知乎登录票据无效或已过期")
		return
	}
	ok(c, tokens)
}

type zhihuTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Code        int    `json:"code"`
	Data        any    `json:"data"`
}

func (a *App) exchangeZhihuAccessToken(ctx context.Context, code string) (string, error) {
	form := url.Values{}
	form.Set("app_id", a.cfg.ZhihuAppID)
	form.Set("app_key", a.cfg.ZhihuAppKey)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", a.cfg.ZhihuRedirectURI)
	form.Set("code", code)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+normalizeHost(a.cfg.ZhihuOpenAPIHost)+"/access_token", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求知乎 token 失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("知乎 token 接口返回 %d", resp.StatusCode)
	}
	var parsed zhihuTokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("知乎 token 响应解析失败: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("知乎 token 响应无 access_token")
	}
	return parsed.AccessToken, nil
}

type zhihuUserInfo struct {
	UID         json.Number `json:"uid"`
	Fullname    string      `json:"fullname"`
	Gender      string      `json:"gender"`
	Headline    string      `json:"headline"`
	Description string      `json:"description"`
	AvatarPath  string      `json:"avatar_path"`
	PhoneNo     string      `json:"phone_no"`
	Email       string      `json:"email"`
	Code        int         `json:"code"`
	Data        any         `json:"data"`
}

func (a *App) fetchZhihuUser(ctx context.Context, accessToken string) (zhihuUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+normalizeHost(a.cfg.ZhihuOpenAPIHost)+"/user", nil)
	if err != nil {
		return zhihuUserInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return zhihuUserInfo{}, fmt.Errorf("请求知乎用户信息失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zhihuUserInfo{}, fmt.Errorf("知乎用户信息接口返回 %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var parsed zhihuUserInfo
	if err := decoder.Decode(&parsed); err != nil {
		return zhihuUserInfo{}, fmt.Errorf("知乎用户信息响应解析失败: %w", err)
	}
	if parsed.Code >= 400 {
		return zhihuUserInfo{}, fmt.Errorf("知乎用户信息接口拒绝访问: %v", parsed.Data)
	}
	if parsed.UID.String() == "" {
		return zhihuUserInfo{}, errors.New("知乎用户信息缺少 uid")
	}
	return parsed, nil
}

func (a *App) upsertUserFromZhihu(zu zhihuUserInfo) (User, error) {
	uid := zu.UID.String()
	now := time.Now()
	var u User
	err := a.db.First(&u, "zhihu_uid = ?", uid).Error
	if err == nil {
		u.LastLoginAt = &now
		return u, a.db.Model(&u).Update("last_login_at", now).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(randomToken()), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	u = User{
		ID:           newID(),
		Username:     a.uniqueOAuthUsername(uid),
		ZhihuUID:     &uid,
		Role:         roleUser,
		Enabled:      true,
		PasswordHash: string(hash),
		CreatedAt:    now,
		LastLoginAt:  &now,
	}
	if err := a.db.Create(&u).Error; err != nil {
		return User{}, err
	}
	return u, nil
}

func (a *App) uniqueOAuthUsername(uid string) string {
	base := "zhihu_" + strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == '-' {
			return r
		}
		return -1
	}, uid)
	if len(base) <= 64 {
		var count int64
		a.db.Model(&User{}).Where("username = ?", base).Count(&count)
		if count == 0 {
			return base
		}
	} else {
		base = base[:64]
	}
	for i := 1; ; i++ {
		suffix := fmt.Sprintf("_%d", i)
		candidate := base
		if len(candidate)+len(suffix) > 64 {
			candidate = candidate[:64-len(suffix)]
		}
		candidate += suffix
		var count int64
		a.db.Model(&User{}).Where("username = ?", candidate).Count(&count)
		if count == 0 {
			return candidate
		}
	}
}

func (a *App) redirectOAuthError(c *gin.Context, message string) {
	redirectURL, err := url.Parse(strings.TrimRight(a.cfg.FrontendOrigin, "/") + "/login")
	if err != nil {
		fail(c, http.StatusInternalServerError, message)
		return
	}
	query := redirectURL.Query()
	query.Set("oauth_error", message)
	redirectURL.RawQuery = query.Encode()
	c.Redirect(http.StatusFound, redirectURL.String())
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	return strings.TrimRight(host, "/")
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return newID()
	}
	return hex.EncodeToString(b[:])
}
