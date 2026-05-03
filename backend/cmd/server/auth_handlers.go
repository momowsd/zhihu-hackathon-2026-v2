package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type tokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
}

func (a *App) register(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !bind(c, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Password) < 6 {
		fail(c, http.StatusBadRequest, "用户名至少 3 位，密码至少 6 位")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, http.StatusInternalServerError, "密码处理失败")
		return
	}
	u := User{ID: newID(), Username: req.Username, Role: roleUser, Enabled: true, PasswordHash: string(hash), CreatedAt: time.Now()}
	if err := a.db.Create(&u).Error; err != nil {
		fail(c, http.StatusConflict, "用户名已存在")
		return
	}
	tokens, err := a.issueTokens(u)
	if err != nil {
		fail(c, http.StatusInternalServerError, "签发 token 失败")
		return
	}
	ok(c, tokens)
}

func (a *App) login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !bind(c, &req) {
		return
	}
	var u User
	if err := a.db.Where("username = ?", strings.TrimSpace(req.Username)).First(&u).Error; err != nil || !u.Enabled {
		fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	now := time.Now()
	a.db.Model(&u).Update("last_login_at", now)
	u.LastLoginAt = &now
	tokens, err := a.issueTokens(u)
	if err != nil {
		fail(c, http.StatusInternalServerError, "签发 token 失败")
		return
	}
	ok(c, tokens)
}

func (a *App) refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !bind(c, &req) {
		return
	}
	claims, err := a.parseToken(req.RefreshToken)
	if err != nil || claims["tokenType"] != "refresh" {
		fail(c, http.StatusUnauthorized, "refresh token 无效")
		return
	}
	var u User
	if err := a.db.First(&u, "id = ?", claims["sub"]).Error; err != nil || !u.Enabled {
		fail(c, http.StatusUnauthorized, "用户无效")
		return
	}
	tokens, err := a.issueTokens(u)
	if err != nil {
		fail(c, http.StatusInternalServerError, "签发 token 失败")
		return
	}
	ok(c, tokens)
}

func (a *App) me(c *gin.Context) {
	ok(c, currentUser(c))
}

func (a *App) issueTokens(u User) (tokenPair, error) {
	now := time.Now()
	accessExp := now.Add(time.Hour)
	refreshExp := now.Add(7 * 24 * time.Hour)
	access, err := a.sign(jwt.MapClaims{"sub": u.ID, "username": u.Username, "role": u.Role, "tokenType": "access", "iat": now.Unix(), "exp": accessExp.Unix()})
	if err != nil {
		return tokenPair{}, err
	}
	refresh, err := a.sign(jwt.MapClaims{"sub": u.ID, "username": u.Username, "role": u.Role, "tokenType": "refresh", "iat": now.Unix(), "exp": refreshExp.Unix()})
	return tokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: int64(time.Hour.Seconds())}, err
}

func (a *App) sign(claims jwt.MapClaims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.cfg.JWTSecret))
}

func (a *App) parseToken(raw string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		return []byte(a.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (a *App) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			fail(c, http.StatusUnauthorized, "缺少登录态")
			c.Abort()
			return
		}
		claims, err := a.parseToken(strings.TrimPrefix(header, "Bearer "))
		if err != nil || claims["tokenType"] != "access" {
			fail(c, http.StatusUnauthorized, "登录态无效")
			c.Abort()
			return
		}
		var u User
		if err := a.db.First(&u, "id = ?", fmt.Sprint(claims["sub"])).Error; err != nil || !u.Enabled {
			fail(c, http.StatusUnauthorized, "用户无效")
			c.Abort()
			return
		}
		c.Set("user", u)
		c.Next()
	}
}

func (a *App) adminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if currentUser(c).Role != roleAdmin {
			fail(c, http.StatusForbidden, "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

func currentUser(c *gin.Context) User {
	u, _ := c.Get("user")
	if user, ok := u.(User); ok {
		return user
	}
	return User{}
}
