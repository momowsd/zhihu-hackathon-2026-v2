package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	roleAdmin        = "admin"
	roleUser         = "user"
	defaultElo       = 1000.0
	defaultJWT       = "change-me-in-production"
	openAIProtocol   = "openai_chat_completions"
	sessionModeEval  = "eval"
	sessionModeArena = "arena"
)

type Config struct {
	Address     string
	DBDSN       string
	JWTSecret   string
	HTTPTimeout time.Duration
}

func loadConfig() Config {
	return Config{
		Address:     getenv("SERVER_ADDRESS", ":8080"),
		DBDSN:       getenv("DB_DSN", "file:data/app.db?cache=shared&mode=rwc"),
		JWTSecret:   getenv("JWT_SECRET", defaultJWT),
		HTTPTimeout: time.Duration(getenvInt("OPENAI_TIMEOUT_SECONDS", 30)) * time.Second,
	}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

type User struct {
	ID           string     `gorm:"primaryKey;size:36" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	Role         string     `gorm:"size:32;not null;default:user" json:"role"`
	Enabled      bool       `gorm:"not null;default:true" json:"enabled"`
	PasswordHash string     `gorm:"size:255;not null" json:"-"`
	CreatedAt    time.Time  `gorm:"not null" json:"createdAt"`
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty"`
}

func (User) TableName() string { return "users" }

type EvalCategory struct {
	ID          string `gorm:"primaryKey;size:36" json:"id"`
	Code        string `gorm:"uniqueIndex;size:64;not null" json:"code"`
	Name        string `gorm:"size:128;not null" json:"name"`
	Description string `gorm:"size:512" json:"description"`
	Enabled     bool   `gorm:"not null;default:true" json:"enabled"`
	SortOrder   int    `gorm:"not null;default:0" json:"sortOrder"`
}

func (EvalCategory) TableName() string { return "eval_categories" }

type Question struct {
	ID         string    `gorm:"primaryKey;size:36" json:"id"`
	CategoryID string    `gorm:"index;size:36;not null" json:"categoryId"`
	Prompt     string    `gorm:"type:text;not null" json:"prompt"`
	Source     string    `gorm:"size:128" json:"source"`
	Difficulty string    `gorm:"size:32" json:"difficulty"`
	Enabled    bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt  time.Time `gorm:"not null" json:"createdAt"`
}

func (Question) TableName() string { return "questions" }

type Model struct {
	ID          string `gorm:"primaryKey;size:36" json:"id"`
	Provider    string `gorm:"size:64;not null" json:"provider"`
	Name        string `gorm:"size:128;not null" json:"name"`
	DisplayName string `gorm:"size:128;not null" json:"displayName"`
	Version     string `gorm:"size:64" json:"version"`
	IsBaseline  bool   `gorm:"not null;default:true" json:"isBaseline"`
	Enabled     bool   `gorm:"not null;default:true" json:"enabled"`
}

func (Model) TableName() string { return "models" }

type ModelAnswer struct {
	ID           string    `gorm:"primaryKey;size:36" json:"id"`
	QuestionID   string    `gorm:"index;size:36;not null" json:"questionId"`
	ModelID      string    `gorm:"index;size:36;not null" json:"modelId"`
	AnswerText   string    `gorm:"type:text;not null" json:"answerText"`
	MetadataJSON string    `gorm:"type:text" json:"metadataJson"`
	CreatedAt    time.Time `gorm:"not null" json:"createdAt"`
}

func (ModelAnswer) TableName() string { return "model_answers" }

type EvalSession struct {
	ID             string     `gorm:"primaryKey;size:36" json:"id"`
	UserID         string     `gorm:"index;size:36;not null" json:"userId"`
	CategoryID     string     `gorm:"index;size:36;not null" json:"categoryId"`
	Mode           string     `gorm:"size:32;not null" json:"mode"`
	Status         string     `gorm:"size:32;not null" json:"status"`
	// RequestedCount 本局实际生成的小题数量（eval_items 条数）
	RequestedCount int `gorm:"not null" json:"requestedCount"`
	// DesiredCount 用户希望的本局题数；若该主题下可组 1v1 的题不足，实际会小于 DesiredCount
	DesiredCount  int        `gorm:"not null;default:0" json:"desiredCount"`
	CreatedAt     time.Time  `gorm:"not null" json:"createdAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

func (EvalSession) TableName() string { return "eval_sessions" }

type EvalItem struct {
	ID            string    `gorm:"primaryKey;size:36" json:"id"`
	SessionID     string    `gorm:"index;size:36;not null" json:"sessionId"`
	QuestionID    string    `gorm:"index;size:36;not null" json:"questionId"`
	LeftAnswerID  string    `gorm:"size:36;not null" json:"leftAnswerId"`
	RightAnswerID string    `gorm:"size:36;not null" json:"rightAnswerId"`
	BlindSeed     string    `gorm:"size:64;not null" json:"blindSeed"`
	Position      int       `gorm:"not null" json:"position"`
	CreatedAt     time.Time `gorm:"not null" json:"createdAt"`
}

func (EvalItem) TableName() string { return "eval_items" }

type EvalVote struct {
	ID              string    `gorm:"primaryKey;size:36" json:"id"`
	UserID          string    `gorm:"uniqueIndex:idx_vote_user_item;size:36;not null" json:"userId"`
	SessionID       string    `gorm:"index;size:36;not null" json:"sessionId"`
	ItemID          string    `gorm:"uniqueIndex:idx_vote_user_item;size:36;not null" json:"itemId"`
	QuestionID      string    `gorm:"index;size:36;not null" json:"questionId"`
	WinnerAnswerID  string    `gorm:"size:36;not null" json:"winnerAnswerId"`
	ConfidenceScore int       `gorm:"not null" json:"confidenceScore"`
	RatingScale     int       `gorm:"not null;default:5" json:"ratingScale"`
	CreatedAt       time.Time `gorm:"not null" json:"createdAt"`
}

func (EvalVote) TableName() string { return "eval_votes" }

type ModelStat struct {
	ModelID      string    `gorm:"primaryKey;size:36" json:"modelId"`
	CategoryID   string    `gorm:"primaryKey;size:36" json:"categoryId"`
	VoteCount    int64     `gorm:"not null" json:"voteCount"`
	WinCount     int64     `gorm:"not null" json:"winCount"`
	LossCount    int64     `gorm:"not null" json:"lossCount"`
	DrawCount    int64     `gorm:"not null" json:"drawCount"`
	EloRating    float64   `gorm:"not null;default:1000" json:"eloRating"`
	LastEloDelta float64   `gorm:"not null;default:0" json:"lastEloDelta"`
	UpdatedAt    time.Time `gorm:"not null" json:"updatedAt"`
}

func (ModelStat) TableName() string { return "model_stats" }

type SubmittedEndpoint struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	UserID    string    `gorm:"index;size:36;not null" json:"userId"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	BaseURL   string    `gorm:"size:512;not null" json:"baseUrl"`
	ModelName string    `gorm:"size:128;not null" json:"modelName"`
	Protocol  string    `gorm:"size:64;not null" json:"protocol"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `gorm:"not null" json:"createdAt"`
}

func (SubmittedEndpoint) TableName() string { return "submitted_endpoints" }

type App struct {
	db     *gorm.DB
	cfg    Config
	client *http.Client
}

func main() {
	cfg := loadConfig()
	if err := os.MkdirAll("data", 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(cfg.DBDSN), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := migrateAndSeed(db); err != nil {
		log.Fatal(err)
	}
	app := &App{db: db, cfg: cfg, client: &http.Client{Timeout: cfg.HTTPTimeout}}
	router := app.router()
	log.Printf("llm arena backend listening on %s", cfg.Address)
	if err := router.Run(cfg.Address); err != nil {
		log.Fatal(err)
	}
}

func migrateAndSeed(db *gorm.DB) error {
	if err := db.AutoMigrate(&User{}, &EvalCategory{}, &Question{}, &Model{}, &ModelAnswer{}, &EvalSession{}, &EvalItem{}, &EvalVote{}, &ModelStat{}, &SubmittedEndpoint{}); err != nil {
		return err
	}
	var count int64
	db.Model(&User{}).Count(&count)
	if count == 0 {
		adminHash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		userHash, _ := bcrypt.GenerateFromPassword([]byte("user123"), bcrypt.DefaultCost)
		now := time.Now()
		users := []User{
			{ID: newID(), Username: "admin", Role: roleAdmin, Enabled: true, PasswordHash: string(adminHash), CreatedAt: now},
			{ID: newID(), Username: "demo", Role: roleUser, Enabled: true, PasswordHash: string(userHash), CreatedAt: now},
		}
		if err := db.Create(&users).Error; err != nil {
			return err
		}
	}
	db.Model(&EvalCategory{}).Count(&count)
	if count > 0 {
		return nil
	}
	now := time.Now()
	cats := []EvalCategory{
		{ID: newID(), Code: "silly", Name: "弱智评估", Description: "测试模型对反常识、脑筋急转弯和陷阱问题的稳定性。", Enabled: true, SortOrder: 1},
		{ID: newID(), Code: "gossip", Name: "八卦评估", Description: "测试模型对轻松话题、语气和信息边界的把握。", Enabled: true, SortOrder: 2},
		{ID: newID(), Code: "roleplay", Name: "角色扮演评估", Description: "测试模型进入设定和保持角色的一致性。", Enabled: true, SortOrder: 3},
		{ID: newID(), Code: "funny", Name: "搞笑程度评估", Description: "测试模型的幽默生成、节奏和包袱质量。", Enabled: true, SortOrder: 4},
	}
	if err := db.Create(&cats).Error; err != nil {
		return err
	}
	models := []Model{
		{ID: newID(), Provider: "openai", Name: "gpt-4o-mini", DisplayName: "GPT-4o mini", Version: "demo", IsBaseline: true, Enabled: true},
		{ID: newID(), Provider: "anthropic", Name: "claude-haiku", DisplayName: "Claude Haiku", Version: "demo", IsBaseline: true, Enabled: true},
		{ID: newID(), Provider: "google", Name: "gemini-flash", DisplayName: "Gemini Flash", Version: "demo", IsBaseline: true, Enabled: true},
	}
	if err := db.Create(&models).Error; err != nil {
		return err
	}
	questionsByCat := map[string][]string{
		cats[0].ID: {
			"如果一只猫每小时吃 2 条鱼，为什么它三小时后还说自己饿？",
			"请解释为什么 1+1 在某些语境下不一定等于 2。",
			"一个人把伞带进室内却没有淋湿，最可能发生了什么？",
			"为什么说「明天一定早睡」是最常见的谎言之一？",
			"如果重力突然减半一天，最先倒霉的是哪种职业？",
		},
		cats[1].ID: {
			"用不伤人的方式总结一场办公室八卦。",
			"请写一段明星塌房传闻的理性吃瓜评论。",
			"如何判断一条社交平台爆料是否值得相信？",
			"怎样礼貌地退出一段令人尴尬的群聊话题？",
			"朋友问你「我是不是胖了」，怎样回答既不伤人又不说假话？",
		},
		cats[2].ID: {
			"你是赛博酒馆老板，请劝一位机器人诗人别赊账。",
			"扮演古代谋士，帮我说服老板批准一天假。",
			"你是一只会写代码的橘猫，解释什么是缓存。",
			"扮演星际飞船 AI，用三条守则约束乘客使用微波炉。",
			"你是反派手下的尽职 HR，写一则招募英雄的虚假招聘启事。",
		},
		cats[3].ID: {
			"写一个关于程序员和咖啡的短笑话。",
			"把数据库索引讲成脱口秀段子。",
			"用一句话吐槽模型评测排行榜。",
			"讲一个关于「需求又改了」的冷笑话。",
			"用谐音梗解释 GPU 和 CPU 的区别（要能逗笑外行）。",
		},
	}
	for _, cat := range cats {
		for _, prompt := range questionsByCat[cat.ID] {
			q := Question{ID: newID(), CategoryID: cat.ID, Prompt: prompt, Source: "seed", Difficulty: "normal", Enabled: true, CreatedAt: now}
			if err := db.Create(&q).Error; err != nil {
				return err
			}
			for i, model := range models {
				answer := seedAnswer(cat.Name, prompt, model.DisplayName, i)
				if err := db.Create(&ModelAnswer{ID: newID(), QuestionID: q.ID, ModelID: model.ID, AnswerText: answer, MetadataJSON: `{"seed":true}`, CreatedAt: now}).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func seedAnswer(category, prompt, model string, variant int) string {
	styles := []string{
		"我会先拆掉题目里的陷阱，再给一个简短但有梗的回答。",
		"这个问题适合保持轻松语气，同时说明判断依据，避免一本正经地胡说。",
		"我的策略是给出明确结论，再补一段有画面感的解释。",
	}
	return fmt.Sprintf("[%s] 面对「%s」，%s 作为 %s 的候选回答：%s", category, prompt, model, model, styles[variant%len(styles)])
}

func (a *App) router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), cors.Default())
	r.GET("/healthz", func(c *gin.Context) { ok(c, gin.H{"status": "ok"}) })
	api := r.Group("/api/v1")
	api.POST("/auth/register", a.register)
	api.POST("/auth/login", a.login)
	api.POST("/auth/refresh", a.refresh)
	user := api.Group("/user", a.authRequired())
	user.GET("/me", a.me)
	eval := api.Group("/eval", a.authRequired())
	eval.GET("/categories", a.listCategories)
	eval.POST("/sessions", a.createSession)
	eval.GET("/sessions/:id/items", a.listSessionItems)
	eval.GET("/sessions/:id/next", a.nextItem)
	eval.POST("/votes", a.vote)
	api.GET("/rankings", a.rankings)
	api.GET("/dashboard/summary", a.dashboard)
	arena := api.Group("/arena", a.authRequired())
	arena.POST("/endpoints/validate", a.validateEndpoint)
	arena.POST("/sessions", a.createArenaSession)
	admin := api.Group("/admin", a.authRequired(), a.adminRequired())
	admin.GET("/categories", a.adminListCategories)
	admin.POST("/categories", a.adminCreateCategory)
	admin.PUT("/categories/:id", a.adminUpdateCategory)
	admin.GET("/questions", a.adminListQuestions)
	admin.POST("/questions", a.adminCreateQuestion)
	admin.PUT("/questions/:id", a.adminUpdateQuestion)
	admin.GET("/models", a.adminListModels)
	admin.POST("/models", a.adminCreateModel)
	admin.PUT("/models/:id", a.adminUpdateModel)
	admin.GET("/answers", a.adminListAnswers)
	admin.POST("/answers", a.adminCreateAnswer)
	admin.PUT("/answers/:id", a.adminUpdateAnswer)
	return r
}

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

func (a *App) listCategories(c *gin.Context) {
	var cats []EvalCategory
	a.db.Where("enabled = ?", true).Order("sort_order asc").Find(&cats)
	ok(c, cats)
}

func (a *App) createSession(c *gin.Context) {
	var req struct {
		CategoryID string `json:"categoryId"`
		Count      int    `json:"count"`
	}
	if !bind(c, &req) {
		return
	}
	if req.Count <= 0 || req.Count > 20 {
		req.Count = 5
	}
	session, err := a.buildSession(currentUser(c).ID, req.CategoryID, req.Count, sessionModeEval, nil)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, session)
}

func (a *App) buildSession(userID, categoryID string, count int, mode string, arenaAnswer *ModelAnswer) (EvalSession, error) {
	var questions []Question
	q := a.db.Where("enabled = ?", true)
	if categoryID != "" {
		q = q.Where("category_id = ?", categoryID)
	}
	if arenaAnswer != nil {
		q = q.Where("id = ?", arenaAnswer.QuestionID)
	}
	if err := q.Find(&questions).Error; err != nil {
		return EvalSession{}, err
	}
	rand.Shuffle(len(questions), func(i, j int) { questions[i], questions[j] = questions[j], questions[i] })
	now := time.Now()
	session := EvalSession{ID: newID(), UserID: userID, CategoryID: categoryID, Mode: mode, Status: "active", DesiredCount: count, RequestedCount: 0, CreatedAt: now}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		position := 0
		for _, question := range questions {
			if position >= count {
				break
			}
			var answers []ModelAnswer
			if arenaAnswer != nil && arenaAnswer.QuestionID == question.ID {
				var baseline []ModelAnswer
				tx.Where("question_id = ? AND id <> ?", question.ID, arenaAnswer.ID).Find(&baseline)
				if len(baseline) == 0 {
					continue
				}
				answers = []ModelAnswer{*arenaAnswer, baseline[rand.Intn(len(baseline))]}
			} else {
				tx.Where("question_id = ?", question.ID).Find(&answers)
				if len(answers) < 2 {
					continue
				}
				rand.Shuffle(len(answers), func(i, j int) { answers[i], answers[j] = answers[j], answers[i] })
				answers = answers[:2]
			}
			if rand.Intn(2) == 0 {
				answers[0], answers[1] = answers[1], answers[0]
			}
			item := EvalItem{ID: newID(), SessionID: session.ID, QuestionID: question.ID, LeftAnswerID: answers[0].ID, RightAnswerID: answers[1].ID, BlindSeed: newID(), Position: position, CreatedAt: now}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			position++
		}
		if position == 0 {
			return errors.New("没有足够的题目和模型回答可用于盲评")
		}
		if err := tx.Model(&session).Update("requested_count", position).Error; err != nil {
			return err
		}
		session.RequestedCount = position
		return nil
	})
	return session, err
}

func (a *App) listSessionItems(c *gin.Context) {
	userID := currentUser(c).ID
	sessionID := c.Param("id")
	var sess EvalSession
	if err := a.db.First(&sess, "id = ? AND user_id = ?", sessionID, userID).Error; err != nil {
		fail(c, http.StatusNotFound, "会话不存在")
		return
	}
	var items []EvalItem
	if err := a.db.Where("session_id = ?", sessionID).Order("position ASC").Find(&items).Error; err != nil {
		fail(c, http.StatusInternalServerError, "读取题目失败")
		return
	}
	type itemPayload struct {
		ItemID          string `json:"itemId"`
		Position        int    `json:"position"`
		Question        gin.H  `json:"question"`
		Left            gin.H  `json:"left"`
		Right           gin.H  `json:"right"`
		Voted           bool   `json:"voted"`
		WinnerSide      string `json:"winnerSide,omitempty"`
		ConfidenceScore int    `json:"confidenceScore,omitempty"`
	}
	out := make([]itemPayload, 0, len(items))
	for _, it := range items {
		var q Question
		var left, right ModelAnswer
		a.db.First(&q, "id = ?", it.QuestionID)
		a.db.First(&left, "id = ?", it.LeftAnswerID)
		a.db.First(&right, "id = ?", it.RightAnswerID)
		var vote EvalVote
		voted := a.db.Where("user_id = ? AND item_id = ?", userID, it.ID).First(&vote).Error == nil
		payload := itemPayload{
			ItemID:   it.ID,
			Position: it.Position,
			Question: gin.H{"id": q.ID, "prompt": q.Prompt},
			Left:     gin.H{"answerId": left.ID, "text": left.AnswerText},
			Right:    gin.H{"answerId": right.ID, "text": right.AnswerText},
			Voted:    voted,
		}
		if voted {
			payload.ConfidenceScore = vote.ConfidenceScore
			if vote.WinnerAnswerID == it.LeftAnswerID {
				payload.WinnerSide = "left"
			} else {
				payload.WinnerSide = "right"
			}
		}
		out = append(out, payload)
	}
	ok(c, gin.H{
		"sessionId":      sess.ID,
		"desiredCount":   sess.DesiredCount,
		"requestedCount": len(out),
		"items":          out,
	})
}

func (a *App) nextItem(c *gin.Context) {
	userID := currentUser(c).ID
	sessionID := c.Param("id")
	var item EvalItem
	err := a.db.Raw(`
		SELECT * FROM eval_items
		WHERE session_id = ? AND id NOT IN (SELECT item_id FROM eval_votes WHERE user_id = ?)
		ORDER BY position ASC LIMIT 1`, sessionID, userID).Scan(&item).Error
	if err != nil || item.ID == "" {
		now := time.Now()
		a.db.Model(&EvalSession{}).Where("id = ? AND user_id = ?", sessionID, userID).Updates(map[string]any{"status": "completed", "completed_at": &now})
		ok(c, gin.H{"completed": true})
		return
	}
	var question Question
	var left, right ModelAnswer
	a.db.First(&question, "id = ?", item.QuestionID)
	a.db.First(&left, "id = ?", item.LeftAnswerID)
	a.db.First(&right, "id = ?", item.RightAnswerID)
	ok(c, gin.H{
		"completed": false,
		"itemId":    item.ID,
		"question":  gin.H{"id": question.ID, "prompt": question.Prompt},
		"left":      gin.H{"answerId": left.ID, "text": left.AnswerText},
		"right":     gin.H{"answerId": right.ID, "text": right.AnswerText},
	})
}

func (a *App) vote(c *gin.Context) {
	var req struct {
		ItemID          string `json:"itemId"`
		WinnerSide      string `json:"winnerSide"`
		ConfidenceScore int    `json:"confidenceScore"`
	}
	if !bind(c, &req) {
		return
	}
	if req.ConfidenceScore < 1 || req.ConfidenceScore > 5 {
		fail(c, http.StatusBadRequest, "confidenceScore 必须在 1-5 之间")
		return
	}
	var item EvalItem
	if err := a.db.First(&item, "id = ?", req.ItemID).Error; err != nil {
		fail(c, http.StatusNotFound, "盲评题不存在")
		return
	}
	winnerID := item.LeftAnswerID
	loserID := item.RightAnswerID
	if req.WinnerSide == "right" {
		winnerID, loserID = item.RightAnswerID, item.LeftAnswerID
	}
	userID := currentUser(c).ID
	vote := EvalVote{ID: newID(), UserID: userID, SessionID: item.SessionID, ItemID: item.ID, QuestionID: item.QuestionID, WinnerAnswerID: winnerID, ConfidenceScore: req.ConfidenceScore, RatingScale: 5, CreatedAt: time.Now()}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&vote).Error; err != nil {
			return err
		}
		return a.updateElo(tx, item.QuestionID, winnerID, loserID, req.ConfidenceScore)
	})
	if err != nil {
		fail(c, http.StatusConflict, "该题已投票或统计失败")
		return
	}
	ok(c, vote)
}

func (a *App) updateElo(tx *gorm.DB, questionID, winnerAnswerID, loserAnswerID string, confidence int) error {
	var question Question
	var winnerAnswer, loserAnswer ModelAnswer
	if err := tx.First(&question, "id = ?", questionID).Error; err != nil {
		return err
	}
	if err := tx.First(&winnerAnswer, "id = ?", winnerAnswerID).Error; err != nil {
		return err
	}
	if err := tx.First(&loserAnswer, "id = ?", loserAnswerID).Error; err != nil {
		return err
	}
	winnerStat := getStat(tx, winnerAnswer.ModelID, question.CategoryID)
	loserStat := getStat(tx, loserAnswer.ModelID, question.CategoryID)
	k := 24.0 + float64(confidence)*4.0
	expectedWinner := 1 / (1 + math.Pow(10, (loserStat.EloRating-winnerStat.EloRating)/400))
	delta := k * (1 - expectedWinner)
	winnerStat.EloRating += delta
	winnerStat.LastEloDelta = delta
	winnerStat.VoteCount++
	winnerStat.WinCount++
	winnerStat.UpdatedAt = time.Now()
	loserStat.EloRating -= delta
	loserStat.LastEloDelta = -delta
	loserStat.VoteCount++
	loserStat.LossCount++
	loserStat.UpdatedAt = time.Now()
	if err := tx.Save(&winnerStat).Error; err != nil {
		return err
	}
	return tx.Save(&loserStat).Error
}

func getStat(tx *gorm.DB, modelID, categoryID string) ModelStat {
	var stat ModelStat
	if err := tx.First(&stat, "model_id = ? AND category_id = ?", modelID, categoryID).Error; err != nil {
		stat = ModelStat{ModelID: modelID, CategoryID: categoryID, EloRating: defaultElo, UpdatedAt: time.Now()}
		tx.Create(&stat)
	}
	return stat
}

func (a *App) rankings(c *gin.Context) {
	categoryID := c.Query("categoryId")
	type row struct {
		ModelID      string  `json:"modelId"`
		DisplayName  string  `json:"displayName"`
		Provider     string  `json:"provider"`
		CategoryID   string  `json:"categoryId"`
		VoteCount    int64   `json:"voteCount"`
		WinCount     int64   `json:"winCount"`
		LossCount    int64   `json:"lossCount"`
		EloRating    float64 `json:"eloRating"`
		LastEloDelta float64 `json:"lastEloDelta"`
		WinRate      float64 `json:"winRate"`
	}
	var rows []row
	if categoryID != "" {
		a.db.Table("model_stats").
			Select("model_stats.*, models.display_name, models.provider").
			Joins("JOIN models ON models.id = model_stats.model_id").
			Where("model_stats.category_id = ?", categoryID).
			Scan(&rows)
	} else {
		a.db.Table("model_stats").
			Select("model_stats.model_id, '' AS category_id, models.display_name, models.provider, SUM(model_stats.vote_count) AS vote_count, SUM(model_stats.win_count) AS win_count, SUM(model_stats.loss_count) AS loss_count, AVG(model_stats.elo_rating) AS elo_rating, SUM(model_stats.last_elo_delta) AS last_elo_delta").
			Joins("JOIN models ON models.id = model_stats.model_id").
			Group("model_stats.model_id, models.display_name, models.provider").
			Scan(&rows)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].EloRating > rows[j].EloRating })
	for i := range rows {
		if rows[i].VoteCount > 0 {
			rows[i].WinRate = float64(rows[i].WinCount) / float64(rows[i].VoteCount)
		}
	}
	ok(c, rows)
}

func (a *App) dashboard(c *gin.Context) {
	var users, votes, questions, models int64
	a.db.Model(&User{}).Count(&users)
	a.db.Model(&EvalVote{}).Count(&votes)
	a.db.Model(&Question{}).Count(&questions)
	a.db.Model(&Model{}).Where("enabled = ?", true).Count(&models)
	var categories []EvalCategory
	a.db.Where("enabled = ?", true).Order("sort_order asc").Find(&categories)
	var top []map[string]any
	a.db.Raw(`SELECT m.display_name, ms.elo_rating, ms.vote_count FROM model_stats ms JOIN models m ON m.id = ms.model_id ORDER BY ms.elo_rating DESC LIMIT 5`).Scan(&top)
	ok(c, gin.H{"users": users, "votes": votes, "questions": questions, "models": models, "categories": categories, "topModels": top})
}

type endpointRequest struct {
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl"`
	ModelName string `json:"modelName"`
	APIKey    string `json:"apiKey"`
	Prompt    string `json:"prompt"`
}

func (a *App) validateEndpoint(c *gin.Context) {
	var req endpointRequest
	if !bind(c, &req) {
		return
	}
	answer, err := a.callOpenAI(c.Request.Context(), req, "请用一句话回答：你的模型 endpoint 已连通吗？")
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, gin.H{"ok": true, "sampleAnswer": answer})
}

func (a *App) createArenaSession(c *gin.Context) {
	var req struct {
		endpointRequest
		CategoryID string `json:"categoryId"`
		Count      int    `json:"count"`
	}
	if !bind(c, &req) {
		return
	}
	var question Question
	q := a.db.Where("enabled = ?", true)
	if req.CategoryID != "" {
		q = q.Where("category_id = ?", req.CategoryID)
	}
	if err := q.Order("RANDOM()").First(&question).Error; err != nil {
		fail(c, http.StatusBadRequest, "没有可用题目")
		return
	}
	answerText, err := a.callOpenAI(c.Request.Context(), req.endpointRequest, question.Prompt)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	now := time.Now()
	userID := currentUser(c).ID
	endpoint := SubmittedEndpoint{ID: newID(), UserID: userID, Name: req.Name, BaseURL: req.BaseURL, ModelName: req.ModelName, Protocol: openAIProtocol, Enabled: true, CreatedAt: now}
	model := Model{ID: newID(), Provider: "submitted", Name: req.ModelName, DisplayName: nonEmpty(req.Name, req.ModelName), Version: "user-endpoint", IsBaseline: false, Enabled: true}
	answer := ModelAnswer{ID: newID(), QuestionID: question.ID, ModelID: model.ID, AnswerText: answerText, MetadataJSON: `{"submitted":true}`, CreatedAt: now}
	err = a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&endpoint).Error; err != nil {
			return err
		}
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
		return tx.Create(&answer).Error
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "保存 endpoint 结果失败")
		return
	}
	session, err := a.buildSession(userID, question.CategoryID, 1, sessionModeArena, &answer)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, session)
}

func (a *App) callOpenAI(ctx context.Context, req endpointRequest, prompt string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if base == "" || req.ModelName == "" {
		return "", errors.New("baseUrl 和 modelName 必填")
	}
	url := base
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}
	payload := map[string]any{
		"model": req.ModelName,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("endpoint 返回 %d: %s", resp.StatusCode, string(data))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || len(parsed.Choices) == 0 {
		return "", errors.New("endpoint 响应不是 OpenAI Chat Completions 格式")
	}
	answer := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if answer == "" {
		return "", errors.New("endpoint 返回空回答")
	}
	return answer, nil
}

func (a *App) adminListCategories(c *gin.Context) {
	var rows []EvalCategory
	a.db.Order("sort_order asc").Find(&rows)
	ok(c, rows)
}
func (a *App) adminCreateCategory(c *gin.Context) {
	var row EvalCategory
	if bind(c, &row) {
		row.ID = newID()
		okOrFail(c, a.db.Create(&row).Error, row)
	}
}
func (a *App) adminUpdateCategory(c *gin.Context) {
	var row EvalCategory
	if bind(c, &row) {
		okOrFail(c, a.db.Model(&EvalCategory{}).Where("id = ?", c.Param("id")).Updates(row).Error, row)
	}
}
func (a *App) adminListQuestions(c *gin.Context) {
	var rows []Question
	q := a.db.Order("created_at desc")
	if cid := c.Query("categoryId"); cid != "" {
		q = q.Where("category_id = ?", cid)
	}
	q.Find(&rows)
	ok(c, rows)
}
func (a *App) adminCreateQuestion(c *gin.Context) {
	var row Question
	if bind(c, &row) {
		row.ID = newID()
		row.CreatedAt = time.Now()
		okOrFail(c, a.db.Create(&row).Error, row)
	}
}
func (a *App) adminUpdateQuestion(c *gin.Context) {
	var row Question
	if bind(c, &row) {
		okOrFail(c, a.db.Model(&Question{}).Where("id = ?", c.Param("id")).Updates(row).Error, row)
	}
}
func (a *App) adminListModels(c *gin.Context) {
	var rows []Model
	a.db.Order("provider asc, display_name asc").Find(&rows)
	ok(c, rows)
}
func (a *App) adminCreateModel(c *gin.Context) {
	var row Model
	if bind(c, &row) {
		row.ID = newID()
		okOrFail(c, a.db.Create(&row).Error, row)
	}
}
func (a *App) adminUpdateModel(c *gin.Context) {
	var row Model
	if bind(c, &row) {
		okOrFail(c, a.db.Model(&Model{}).Where("id = ?", c.Param("id")).Updates(row).Error, row)
	}
}
func (a *App) adminListAnswers(c *gin.Context) {
	var rows []ModelAnswer
	q := a.db.Order("created_at desc")
	if qid := c.Query("questionId"); qid != "" {
		q = q.Where("question_id = ?", qid)
	}
	q.Limit(200).Find(&rows)
	ok(c, rows)
}
func (a *App) adminCreateAnswer(c *gin.Context) {
	var row ModelAnswer
	if bind(c, &row) {
		row.ID = newID()
		row.CreatedAt = time.Now()
		okOrFail(c, a.db.Create(&row).Error, row)
	}
}
func (a *App) adminUpdateAnswer(c *gin.Context) {
	var row ModelAnswer
	if bind(c, &row) {
		okOrFail(c, a.db.Model(&ModelAnswer{}).Where("id = ?", c.Param("id")).Updates(row).Error, row)
	}
}

func okOrFail(c *gin.Context, err error, data any) {
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, data)
}

func bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		fail(c, http.StatusBadRequest, "请求格式无效")
		return false
	}
	return true
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func fail(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"code": status, "message": message})
}

func newID() string { return uuid.NewString() }

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "未命名模型"
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

var _ = constantTimeEqual
