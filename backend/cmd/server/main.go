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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
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

// loadEnvFiles reads optional .env files (repo root then backend cwd). Does not override variables already set in the environment.
func loadEnvFiles() {
	for _, path := range []string{"../.env", ".env"} {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := godotenv.Load(path); err != nil {
			log.Printf("warning: load env file %s: %v", path, err)
		}
	}
}

type Config struct {
	Address          string
	DBDSN            string
	JWTSecret        string
	HTTPTimeout      time.Duration
	ZhihuAppID       string
	ZhihuAppKey      string
	ZhihuRedirectURI string
	ZhihuOpenAPIHost string
	FrontendOrigin   string
}

func loadConfig() Config {
	return Config{
		Address:          getenv("SERVER_ADDRESS", ":8080"),
		DBDSN:            getenv("DB_DSN", "file:data/app.db?cache=shared&mode=rwc"),
		JWTSecret:        getenv("JWT_SECRET", defaultJWT),
		HTTPTimeout:      time.Duration(getenvInt("OPENAI_TIMEOUT_SECONDS", 30)) * time.Second,
		ZhihuAppID:       getenv("ZHIHU_APP_ID", ""),
		ZhihuAppKey:      getenv("ZHIHU_APP_KEY", ""),
		ZhihuRedirectURI: getenv("ZHIHU_REDIRECT_URI", "http://localhost:8080/api/v1/auth/zhihu/callback"),
		ZhihuOpenAPIHost: getenv("ZHIHU_OPENAPI_HOST", "openapi.zhihu.com"),
		FrontendOrigin:   getenv("FRONTEND_ORIGIN", "http://localhost:5180"),
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
	ZhihuUID     *string    `gorm:"size:64" json:"zhihuUid,omitempty"`
	Role         string     `gorm:"size:32;not null;default:user" json:"role"`
	Enabled      bool       `gorm:"not null;default:true" json:"enabled"`
	PasswordHash string     `gorm:"size:255;not null" json:"-"`
	CreatedAt    time.Time  `gorm:"not null" json:"createdAt"`
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty"`
}

func (User) TableName() string { return "users" }

type EvalCategory struct {
	ID             string `gorm:"primaryKey;size:36" json:"id"`
	Code           string `gorm:"uniqueIndex;size:64;not null" json:"code"`
	Name           string `gorm:"size:128;not null" json:"name"`
	Description    string `gorm:"size:512" json:"description"`
	Enabled        bool   `gorm:"not null;default:true" json:"enabled"`
	SortOrder      int    `gorm:"not null;default:0" json:"sortOrder"`
	DomainSlug     string `gorm:"-" json:"domainSlug,omitempty"`
	SystemPromptMD string `gorm:"-" json:"systemPromptMd,omitempty"`
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
	ID         string `gorm:"primaryKey;size:36" json:"id"`
	UserID     string `gorm:"index;size:36;not null" json:"userId"`
	CategoryID string `gorm:"index;size:36;not null" json:"categoryId"`
	Mode       string `gorm:"size:32;not null" json:"mode"`
	Status     string `gorm:"size:32;not null" json:"status"`
	// RequestedCount 本局实际生成的小题数量（eval_items 条数）
	RequestedCount int `gorm:"not null" json:"requestedCount"`
	// DesiredCount 用户希望的本局题数；若该主题下可组 1v1 的题不足，实际会小于 DesiredCount
	DesiredCount int        `gorm:"not null;default:0" json:"desiredCount"`
	CreatedAt    time.Time  `gorm:"not null" json:"createdAt"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
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
	ID             string `gorm:"primaryKey;size:36" json:"id"`
	UserID         string `gorm:"uniqueIndex:idx_vote_user_item;size:36;not null" json:"userId"`
	SessionID      string `gorm:"index;size:36;not null" json:"sessionId"`
	ItemID         string `gorm:"uniqueIndex:idx_vote_user_item;size:36;not null" json:"itemId"`
	QuestionID     string `gorm:"index;size:36;not null" json:"questionId"`
	WinnerAnswerID string `gorm:"size:36;not null" json:"winnerAnswerId"`
	// VoteOutcome: left | right | both_good | both_bad。历史数据为空时按 winner_answer_id 与左右答案推断 left/right。
	VoteOutcome string `gorm:"size:24;not null;default:''" json:"voteOutcome"`
	// VoteEffectJSON 记录本次投票造成的 Elo 与分类排名前后快照，用于展示单次用户选择贡献。
	VoteEffectJSON string `gorm:"type:text;not null;default:''" json:"voteEffectJson"`
	// ConfidenceScore 历史为 1–5 信心分；现由盲评四档写入档位编码（见 voteTierStoredScore），不再来自请求体。
	ConfidenceScore int       `gorm:"not null" json:"confidenceScore"`
	RatingScale     int       `gorm:"not null;default:5" json:"ratingScale"`
	CreatedAt       time.Time `gorm:"not null" json:"createdAt"`
}

func (EvalVote) TableName() string { return "eval_votes" }

type ModelPeerVote struct {
	ID            string    `gorm:"primaryKey;size:36" json:"id"`
	SourceID      string    `gorm:"uniqueIndex;size:128;not null" json:"sourceId"`
	RunID         string    `gorm:"index;size:128" json:"runId"`
	Domain        string    `gorm:"index;size:128;not null" json:"domain"`
	CategoryID    string    `gorm:"index;size:36;not null" json:"categoryId"`
	QueryID       string    `gorm:"size:64;not null" json:"queryId"`
	QuestionID    string    `gorm:"index;size:36;not null" json:"questionId"`
	JudgeModelID  string    `gorm:"index;size:36;not null" json:"judgeModelId"`
	LeftModelID   string    `gorm:"index;size:36;not null" json:"leftModelId"`
	RightModelID  string    `gorm:"index;size:36;not null" json:"rightModelId"`
	LeftAnswerID  string    `gorm:"size:36" json:"leftAnswerId"`
	RightAnswerID string    `gorm:"size:36" json:"rightAnswerId"`
	Outcome       string    `gorm:"size:24;not null" json:"outcome"`
	Score         float64   `gorm:"not null;default:0" json:"score"`
	Confidence    float64   `gorm:"not null;default:0" json:"confidence"`
	Reason        string    `gorm:"type:text" json:"reason"`
	Seed          int64     `gorm:"not null;default:0" json:"seed"`
	Source        string    `gorm:"size:128;not null;default:model-peer-evals" json:"source"`
	Applied       bool      `gorm:"not null;default:false" json:"applied"`
	EffectJSON    string    `gorm:"type:text;not null;default:''" json:"effectJson"`
	CreatedAt     time.Time `gorm:"not null" json:"createdAt"`
}

func (ModelPeerVote) TableName() string { return "model_peer_votes" }

type dashboardTrendPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type voteModelEffect struct {
	Side       string  `json:"side"`
	ModelID    string  `json:"modelId"`
	EloBefore  float64 `json:"eloBefore"`
	EloAfter   float64 `json:"eloAfter"`
	EloDelta   float64 `json:"eloDelta"`
	RankBefore int     `json:"rankBefore"`
	RankAfter  int     `json:"rankAfter"`
	RankDelta  int     `json:"rankDelta"`
}

type voteEffect struct {
	CategoryID string            `json:"categoryId"`
	Models     []voteModelEffect `json:"models"`
}

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
	db           *gorm.DB
	cfg          Config
	client       *http.Client
	oauthStates  *oauthNonceStore
	oauthTickets *oauthTicketStore
}

func main() {
	loadEnvFiles()
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
	app := &App{
		db:           db,
		cfg:          cfg,
		client:       &http.Client{Timeout: cfg.HTTPTimeout},
		oauthStates:  newOAuthNonceStore(),
		oauthTickets: newOAuthTicketStore(),
	}
	router := app.router()
	log.Printf("llm arena backend listening on %s", cfg.Address)
	if err := router.Run(cfg.Address); err != nil {
		log.Fatal(err)
	}
}

func migrateAndSeed(db *gorm.DB) error {
	if err := db.AutoMigrate(&User{}, &EvalCategory{}, &Question{}, &Model{}, &ModelAnswer{}, &EvalSession{}, &EvalItem{}, &EvalVote{}, &ModelPeerVote{}, &ModelStat{}, &SubmittedEndpoint{}); err != nil {
		return err
	}
	// SQLite forbids ADD COLUMN … UNIQUE; apply uniqueness with a standalone index after the column exists.
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_zhihu_uid ON users(zhihu_uid)").Error; err != nil {
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
	if err := ensureDefaultModels(db); err != nil {
		return err
	}
	db.Model(&EvalCategory{}).Count(&count)
	if count > 0 {
		return syncEvalWorkspaceDomains(db)
	}
	if len(loadEvalDomainMetas()) > 0 {
		return syncEvalWorkspaceDomains(db)
	}
	return nil
}

func defaultModels() []Model {
	return []Model{
		{ID: newID(), Provider: "google", Name: "gemini-3p1-pro-preview-20260219-cloudsway", DisplayName: "Gemini 3.1 Pro Preview", Version: "zhihu-gateway", IsBaseline: true, Enabled: true},
		{ID: newID(), Provider: "anthropic", Name: "claude-opus-4p7-cloudsway", DisplayName: "Claude Opus 4.7", Version: "zhihu-gateway", IsBaseline: true, Enabled: true},
		{ID: newID(), Provider: "openai", Name: "gpt-5-5-2026-04-24", DisplayName: "GPT-5.5", Version: "zhihu-gateway", IsBaseline: true, Enabled: true},
		{ID: newID(), Provider: "bytedance", Name: "doubao-seed-2-0-pro", DisplayName: "Doubao Seed 2.0 Pro", Version: "zhihu-gateway", IsBaseline: true, Enabled: true},
		{ID: newID(), Provider: "zhipu", Name: "glm-5p1-baidubce", DisplayName: "GLM 5.1", Version: "zhihu-gateway", IsBaseline: true, Enabled: true},
		{ID: newID(), Provider: "moonshot", Name: "kimi-2-5-baidubce-security", DisplayName: "Kimi 2.5", Version: "zhihu-gateway", IsBaseline: true, Enabled: true},
		{ID: newID(), Provider: "deepseek", Name: "deepseek-v4-pro-baidubce", DisplayName: "DeepSeek V4 Pro", Version: "zhihu-gateway", IsBaseline: true, Enabled: true},
	}
}

func ensureDefaultModels(db *gorm.DB) error {
	for _, model := range defaultModels() {
		var existing Model
		if err := db.First(&existing, "name = ?", model.Name).Error; err == nil {
			updates := map[string]any{
				"provider":     model.Provider,
				"display_name": model.DisplayName,
				"version":      model.Version,
				"is_baseline":  true,
				"enabled":      true,
			}
			if err := db.Model(&Model{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return err
			}
			continue
		}
		if err := db.Create(&model).Error; err != nil {
			return err
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
	api.GET("/auth/zhihu/start", a.zhihuOAuthStart)
	api.GET("/auth/zhihu/callback", a.zhihuOAuthCallback)
	api.POST("/auth/zhihu/exchange", a.zhihuOAuthExchange)
	user := api.Group("/user", a.authRequired())
	user.GET("/me", a.me)
	user.GET("/history", a.userHistory)
	eval := api.Group("/eval", a.authRequired())
	eval.GET("/categories", a.listCategories)
	eval.POST("/sessions", a.createSession)
	eval.GET("/sessions/:id/items", a.listSessionItems)
	eval.GET("/sessions/:id/next", a.nextItem)
	eval.POST("/votes", a.vote)
	api.GET("/rankings/peer-matrix", a.rankingPeerMatrix)
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
	admin.GET("/browse/:table", a.adminBrowseTable)
	admin.GET("/questions/:id/delete-impact", a.adminQuestionDeleteImpactSingle)
	admin.POST("/questions/delete-impact", a.adminQuestionDeleteImpactBatch)
	admin.DELETE("/questions", a.adminDeleteQuestionsBatch)
	admin.DELETE("/questions/:id", a.adminDeleteQuestion)
	admin.POST("/import/bundle", a.adminImportBundle)
	return r
}

type evalDomainMeta struct {
	Slug            string
	Name            string
	Description     string
	SystemPrompt    string
	RawQueries      []evalRawQuery
	ResponseAnswers map[string]map[string]evalResponseJSON
}

type evalDomainJSON struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type rawQueryJSON struct {
	ID    string `json:"id"`
	Query string `json:"query"`
}

type evalRawQuery struct {
	ID    string
	Query string
}

type evalResponseJSON struct {
	QueryID  string `json:"queryId"`
	ModelID  string `json:"modelId"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl"`
	CalledAt string `json:"calledAt"`
	Answer   string `json:"answer"`
	Error    any    `json:"error"`
}

type peerVoteSideJSON struct {
	ModelID string `json:"modelId"`
}

type peerVoteFileRow struct {
	ID            string            `json:"id"`
	SchemaVersion int               `json:"schemaVersion"`
	RunID         string            `json:"runId"`
	Domain        string            `json:"domain"`
	QueryID       string            `json:"queryId"`
	QuestionID    string            `json:"questionId"`
	JudgeModel    string            `json:"judgeModel"`
	LeftModel     string            `json:"leftModel"`
	RightModel    string            `json:"rightModel"`
	Left          peerVoteSideJSON  `json:"left"`
	Right         peerVoteSideJSON  `json:"right"`
	Outcome       string            `json:"outcome"`
	Score         *float64          `json:"score"`
	Confidence    *float64          `json:"confidence"`
	Reason        string            `json:"reason"`
	Seed          int64             `json:"seed"`
	Metadata      map[string]string `json:"metadata"`
}

func evalWorkspaceRoot() string {
	if root := strings.TrimSpace(os.Getenv("EVAL_WORKSPACE_DOMAINS")); root != "" {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root
		}
	}
	for _, root := range []string{"eval-workspace/domains", "../eval-workspace/domains"} {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root
		}
	}
	return "eval-workspace/domains"
}

func modelPeerEvalRoot() string {
	if root := strings.TrimSpace(os.Getenv("EVAL_MODEL_PEER_EVALS")); root != "" {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root
		}
	}
	domainRoot := evalWorkspaceRoot()
	for _, root := range []string{
		filepath.Join(filepath.Dir(domainRoot), "model-peer-evals"),
		"eval-workspace/model-peer-evals",
		"../eval-workspace/model-peer-evals",
	} {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root
		}
	}
	return filepath.Join(filepath.Dir(domainRoot), "model-peer-evals")
}

func preferredEvalDomainOrder() []string {
	return []string{"ruozhi-eval", "novel-writing-eval", "movie-script-eval", "emotion-eval"}
}

func evalDomainAliases() map[string]string {
	return map[string]string{
		"ruozhi-eval":        "ruozhi-eval",
		"silly":              "ruozhi-eval",
		"弱智评估":               "ruozhi-eval",
		"弱智吧Case评估":          "ruozhi-eval",
		"novel-writing-eval": "novel-writing-eval",
		"novel":              "novel-writing-eval",
		"小说创作评估":             "novel-writing-eval",
		"movie-script-eval":  "movie-script-eval",
		"短剧脚本生成":             "movie-script-eval",
		"emotion-eval":       "emotion-eval",
		"高情商回复":              "emotion-eval",
	}
}

func domainSlugForCategory(cat EvalCategory) string {
	aliases := evalDomainAliases()
	if slug := aliases[strings.TrimSpace(cat.Code)]; slug != "" {
		return slug
	}
	return aliases[strings.TrimSpace(cat.Name)]
}

func readEvalDomainPrompt(slug string) string {
	if strings.TrimSpace(slug) == "" {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(evalWorkspaceRoot(), slug, "prompts", "system.md"))
	if err != nil {
		return ""
	}
	return string(content)
}

func readRawQueries(slug string) []evalRawQuery {
	content, err := os.ReadFile(filepath.Join(evalWorkspaceRoot(), slug, "raw_queries.jsonl"))
	if err != nil {
		return nil
	}
	var queries []evalRawQuery
	seen := map[string]bool{}
	for idx, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row rawQueryJSON
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		query := strings.TrimSpace(row.Query)
		if query == "" || seen[query] {
			continue
		}
		queryID := strings.TrimSpace(row.ID)
		if queryID == "" {
			queryID = fmt.Sprintf("rq-%04d", idx+1)
		}
		seen[query] = true
		queries = append(queries, evalRawQuery{ID: queryID, Query: query})
	}
	return queries
}

func readEvalResponses(slug string) map[string]map[string]evalResponseJSON {
	dir := filepath.Join(evalWorkspaceRoot(), slug, "responses")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := map[string]map[string]evalResponseJSON{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		defaultModelID := strings.TrimSuffix(entry.Name(), ".jsonl")
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var row evalResponseJSON
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				continue
			}
			if row.Error != nil || strings.TrimSpace(row.Answer) == "" {
				continue
			}
			queryID := strings.TrimSpace(row.QueryID)
			if queryID == "" {
				continue
			}
			modelID := strings.TrimSpace(row.ModelID)
			if modelID == "" {
				modelID = strings.TrimSpace(row.Model)
			}
			if modelID == "" {
				modelID = defaultModelID
			}
			row.ModelID = modelID
			row.Answer = strings.TrimSpace(row.Answer)
			if out[queryID] == nil {
				out[queryID] = map[string]evalResponseJSON{}
			}
			out[queryID][modelID] = row
		}
	}
	return out
}

func loadEvalDomainMetas() []evalDomainMeta {
	root := evalWorkspaceRoot()
	out := []evalDomainMeta{}
	for _, slug := range preferredEvalDomainOrder() {
		domainPath := filepath.Join(root, slug, "domain.json")
		content, err := os.ReadFile(domainPath)
		if err != nil {
			continue
		}
		var meta evalDomainJSON
		if err := json.Unmarshal(content, &meta); err != nil {
			continue
		}
		if strings.TrimSpace(meta.Slug) == "" {
			meta.Slug = slug
		}
		out = append(out, evalDomainMeta{
			Slug:            meta.Slug,
			Name:            meta.Name,
			Description:     meta.Description,
			SystemPrompt:    readEvalDomainPrompt(slug),
			RawQueries:      readRawQueries(slug),
			ResponseAnswers: readEvalResponses(slug),
		})
	}
	return out
}

func findCategoryForDomain(db *gorm.DB, slug string) (EvalCategory, error) {
	var cat EvalCategory
	if err := db.First(&cat, "code = ?", slug).Error; err == nil {
		return cat, nil
	}
	for alias, mapped := range evalDomainAliases() {
		if mapped != slug || alias == slug {
			continue
		}
		if err := db.First(&cat, "code = ? OR name = ?", alias, alias).Error; err == nil {
			return cat, nil
		}
	}
	return EvalCategory{}, gorm.ErrRecordNotFound
}

func syncEvalWorkspaceDomains(db *gorm.DB) error {
	domains := loadEvalDomainMetas()
	if len(domains) == 0 {
		return nil
	}
	if err := ensureDefaultModels(db); err != nil {
		return err
	}
	var models []Model
	if err := db.Where("enabled = ?", true).Find(&models).Error; err != nil {
		return err
	}
	modelByName := map[string]Model{}
	for _, model := range models {
		modelByName[strings.TrimSpace(model.Name)] = model
		modelByName[strings.TrimSpace(model.ID)] = model
	}
	activeCodes := make([]string, 0, len(domains))
	now := time.Now()
	for idx, domain := range domains {
		cat, err := findCategoryForDomain(db, domain.Slug)
		if err != nil {
			cat = EvalCategory{ID: newID(), Code: domain.Slug}
			if err := db.Create(&cat).Error; err != nil {
				return err
			}
		}
		updates := map[string]any{
			"code":        domain.Slug,
			"name":        domain.Name,
			"description": domain.Description,
			"enabled":     true,
			"sort_order":  idx + 1,
		}
		if err := db.Model(&EvalCategory{}).Where("id = ?", cat.ID).Updates(updates).Error; err != nil {
			return err
		}
		cat.Code = domain.Slug
		cat.Name = domain.Name
		activeCodes = append(activeCodes, domain.Slug)
		for _, model := range models {
			var stat ModelStat
			if err := db.First(&stat, "model_id = ? AND category_id = ?", model.ID, cat.ID).Error; err == nil {
				continue
			}
			stat = ModelStat{ModelID: model.ID, CategoryID: cat.ID, EloRating: defaultElo, UpdatedAt: now}
			if err := db.Create(&stat).Error; err != nil {
				return err
			}
		}
		for _, raw := range domain.RawQueries {
			prompt := raw.Query
			var q Question
			if err := db.First(&q, "category_id = ? AND prompt = ?", cat.ID, prompt).Error; err != nil {
				q = Question{ID: newID(), CategoryID: cat.ID, Prompt: prompt, Source: "eval-workspace", Difficulty: "normal", Enabled: true, CreatedAt: now}
				if err := db.Create(&q).Error; err != nil {
					return err
				}
			} else if !q.Enabled || q.Source != "eval-workspace" {
				if err := db.Model(&Question{}).Where("id = ?", q.ID).Updates(map[string]any{"source": "eval-workspace", "enabled": true}).Error; err != nil {
					return err
				}
			}
			for modelKey, response := range domain.ResponseAnswers[raw.ID] {
				model, ok := modelByName[strings.TrimSpace(modelKey)]
				if !ok {
					continue
				}
				metadata := map[string]any{
					"source":   "eval-workspace-response",
					"domain":   domain.Slug,
					"queryId":  raw.ID,
					"modelId":  response.ModelID,
					"provider": response.Provider,
					"baseUrl":  response.BaseURL,
					"calledAt": response.CalledAt,
				}
				metadataJSON, _ := json.Marshal(metadata)
				var existing ModelAnswer
				if err := db.First(&existing, "question_id = ? AND model_id = ?", q.ID, model.ID).Error; err == nil {
					if err := db.Model(&ModelAnswer{}).Where("id = ?", existing.ID).Updates(map[string]any{
						"answer_text":   response.Answer,
						"metadata_json": string(metadataJSON),
					}).Error; err != nil {
						return err
					}
					continue
				}
				if err := db.Create(&ModelAnswer{
					ID:           newID(),
					QuestionID:   q.ID,
					ModelID:      model.ID,
					AnswerText:   response.Answer,
					MetadataJSON: string(metadataJSON),
					CreatedAt:    now,
				}).Error; err != nil {
					return err
				}
			}
		}
	}
	if err := db.Model(&EvalCategory{}).Where("code NOT IN ?", activeCodes).Updates(map[string]any{"enabled": false}).Error; err != nil {
		return err
	}
	return syncModelPeerVotes(db, domains)
}

func peerVoteScore(outcome string) float64 {
	switch outcome {
	case "left":
		return 1
	case "right":
		return -1
	case "both_good":
		return 0.35
	case "both_bad":
		return 0
	default:
		return 0
	}
}

func peerTargetScore(outcome string, targetIsLeft bool) float64 {
	switch outcome {
	case "left":
		if targetIsLeft {
			return 1
		}
		return -1
	case "right":
		if targetIsLeft {
			return -1
		}
		return 1
	case "both_good":
		return 0.35
	case "both_bad":
		return 0
	default:
		return 0
	}
}

func peerVoteSourceID(row peerVoteFileRow) string {
	if id := strings.TrimSpace(row.ID); id != "" {
		return id
	}
	queryID := strings.TrimSpace(row.QueryID)
	if queryID == "" {
		queryID = strings.TrimSpace(row.QuestionID)
	}
	return strings.Join([]string{
		strings.TrimSpace(row.RunID),
		strings.TrimSpace(row.Domain),
		queryID,
		strings.TrimSpace(row.JudgeModel),
		strings.TrimSpace(row.LeftModel),
		strings.TrimSpace(row.RightModel),
		strconv.FormatInt(row.Seed, 10),
	}, ":")
}

func loadPeerVoteTasks(root string) (map[string]peerVoteFileRow, error) {
	path := filepath.Join(root, "peer_vote_tasks.jsonl")
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]peerVoteFileRow{}, nil
		}
		return nil, err
	}
	out := map[string]peerVoteFileRow{}
	for lineNo, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row peerVoteFileRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo+1, err)
		}
		if id := strings.TrimSpace(row.ID); id != "" {
			out[id] = row
		}
	}
	return out, nil
}

func mergePeerVoteTask(row peerVoteFileRow, task peerVoteFileRow) peerVoteFileRow {
	if row.SchemaVersion == 0 {
		row.SchemaVersion = task.SchemaVersion
	}
	if strings.TrimSpace(row.RunID) == "" {
		row.RunID = task.RunID
	}
	if strings.TrimSpace(row.Domain) == "" {
		row.Domain = task.Domain
	}
	if strings.TrimSpace(row.QueryID) == "" {
		row.QueryID = task.QueryID
	}
	if strings.TrimSpace(row.QuestionID) == "" {
		row.QuestionID = task.QuestionID
	}
	if strings.TrimSpace(row.JudgeModel) == "" {
		row.JudgeModel = task.JudgeModel
	}
	if strings.TrimSpace(row.LeftModel) == "" {
		row.LeftModel = task.LeftModel
	}
	if strings.TrimSpace(row.RightModel) == "" {
		row.RightModel = task.RightModel
	}
	if strings.TrimSpace(row.Left.ModelID) == "" {
		row.Left = task.Left
	}
	if strings.TrimSpace(row.Right.ModelID) == "" {
		row.Right = task.Right
	}
	if row.Seed == 0 {
		row.Seed = task.Seed
	}
	return row
}

func findQuestionByDomainQuery(db *gorm.DB, cat EvalCategory, domain evalDomainMeta, queryID string) (Question, error) {
	for _, raw := range domain.RawQueries {
		if raw.ID != queryID {
			continue
		}
		var q Question
		if err := db.First(&q, "category_id = ? AND prompt = ?", cat.ID, raw.Query).Error; err != nil {
			return Question{}, err
		}
		return q, nil
	}
	return Question{}, fmt.Errorf("query %s not found in domain %s", queryID, domain.Slug)
}

func findModelAnswerID(db *gorm.DB, questionID, modelID string) string {
	var answer ModelAnswer
	if err := db.First(&answer, "question_id = ? AND model_id = ?", questionID, modelID).Error; err != nil {
		return ""
	}
	return answer.ID
}

func syncModelPeerVotes(db *gorm.DB, domains []evalDomainMeta) error {
	root := modelPeerEvalRoot()
	path := filepath.Join(root, "peer_votes.jsonl")
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	taskRows, err := loadPeerVoteTasks(root)
	if err != nil {
		return err
	}
	domainBySlug := map[string]evalDomainMeta{}
	for _, domain := range domains {
		domainBySlug[domain.Slug] = domain
	}
	var models []Model
	if err := db.Where("enabled = ?", true).Find(&models).Error; err != nil {
		return err
	}
	modelByName := map[string]Model{}
	for _, model := range models {
		modelByName[strings.TrimSpace(model.Name)] = model
		modelByName[strings.TrimSpace(model.ID)] = model
	}

	for lineNo, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row peerVoteFileRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo+1, err)
		}
		if task, ok := taskRows[strings.TrimSpace(row.ID)]; ok {
			row = mergePeerVoteTask(row, task)
		}
		outcome, err := parseEvalVoteOutcome(row.Outcome, "")
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo+1, err)
		}
		if row.Score == nil {
			return fmt.Errorf("%s:%d: missing score", path, lineNo+1)
		}
		if row.Confidence == nil {
			return fmt.Errorf("%s:%d: missing confidence", path, lineNo+1)
		}
		domainSlug := strings.TrimSpace(row.Domain)
		domain, ok := domainBySlug[domainSlug]
		if !ok {
			return fmt.Errorf("%s:%d: unknown domain %s", path, lineNo+1, domainSlug)
		}
		cat, err := findCategoryForDomain(db, domainSlug)
		if err != nil {
			return fmt.Errorf("%s:%d: category not found for domain %s: %w", path, lineNo+1, domainSlug, err)
		}
		queryID := strings.TrimSpace(row.QueryID)
		if queryID == "" {
			queryID = strings.TrimSpace(row.QuestionID)
		}
		question, err := findQuestionByDomainQuery(db, cat, domain, queryID)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo+1, err)
		}
		leftModelKey := strings.TrimSpace(row.LeftModel)
		if leftModelKey == "" {
			leftModelKey = strings.TrimSpace(row.Left.ModelID)
		}
		rightModelKey := strings.TrimSpace(row.RightModel)
		if rightModelKey == "" {
			rightModelKey = strings.TrimSpace(row.Right.ModelID)
		}
		judgeModel, ok := modelByName[strings.TrimSpace(row.JudgeModel)]
		if !ok {
			return fmt.Errorf("%s:%d: unknown judge model %s", path, lineNo+1, row.JudgeModel)
		}
		leftModel, ok := modelByName[leftModelKey]
		if !ok {
			return fmt.Errorf("%s:%d: unknown left model %s", path, lineNo+1, leftModelKey)
		}
		rightModel, ok := modelByName[rightModelKey]
		if !ok {
			return fmt.Errorf("%s:%d: unknown right model %s", path, lineNo+1, rightModelKey)
		}
		if leftModel.ID == rightModel.ID {
			return fmt.Errorf("%s:%d: left and right model must differ", path, lineNo+1)
		}
		score := *row.Score
		sourceID := peerVoteSourceID(row)
		if strings.TrimSpace(sourceID) == "" {
			return fmt.Errorf("%s:%d: missing peer vote id", path, lineNo+1)
		}
		vote := ModelPeerVote{
			ID:            newID(),
			SourceID:      sourceID,
			RunID:         strings.TrimSpace(row.RunID),
			Domain:        domainSlug,
			CategoryID:    cat.ID,
			QueryID:       queryID,
			QuestionID:    question.ID,
			JudgeModelID:  judgeModel.ID,
			LeftModelID:   leftModel.ID,
			RightModelID:  rightModel.ID,
			LeftAnswerID:  findModelAnswerID(db, question.ID, leftModel.ID),
			RightAnswerID: findModelAnswerID(db, question.ID, rightModel.ID),
			Outcome:       outcome,
			Score:         score,
			Confidence:    *row.Confidence,
			Reason:        strings.TrimSpace(row.Reason),
			Seed:          row.Seed,
			Source:        "model-peer-evals",
			CreatedAt:     time.Now(),
		}
		var existing ModelPeerVote
		err = db.First(&existing, "source_id = ?", sourceID).Error
		if err == nil {
			if existing.Applied {
				continue
			}
			vote.ID = existing.ID
			vote.Applied = existing.Applied
			vote.EffectJSON = existing.EffectJSON
			if err := db.Model(&ModelPeerVote{}).Where("id = ?", existing.ID).Updates(vote).Error; err != nil {
				return err
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.Create(&vote).Error; err != nil {
				return err
			}
		} else {
			return err
		}
		if err := applyModelPeerVote(db, vote.ID); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) listCategories(c *gin.Context) {
	var cats []EvalCategory
	a.db.Where("enabled = ?", true).Order("sort_order asc").Find(&cats)
	domainBySlug := map[string]evalDomainMeta{}
	for _, domain := range loadEvalDomainMetas() {
		domainBySlug[domain.Slug] = domain
	}
	out := make([]EvalCategory, 0, len(cats))
	for i := range cats {
		slug := domainSlugForCategory(cats[i])
		cats[i].DomainSlug = slug
		if domain, ok := domainBySlug[slug]; ok {
			if strings.TrimSpace(domain.Name) != "" {
				cats[i].Name = domain.Name
			}
			if strings.TrimSpace(domain.Description) != "" {
				cats[i].Description = domain.Description
			}
		}
		cats[i].SystemPromptMD = readEvalDomainPrompt(slug)
		if strings.TrimSpace(cats[i].SystemPromptMD) != "" {
			out = append(out, cats[i])
		}
	}
	ok(c, out)
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
		ItemID          string      `json:"itemId"`
		Position        int         `json:"position"`
		Question        gin.H       `json:"question"`
		Left            gin.H       `json:"left"`
		Right           gin.H       `json:"right"`
		Voted           bool        `json:"voted"`
		Outcome         string      `json:"outcome,omitempty"`
		WinnerSide      string      `json:"winnerSide,omitempty"`
		ConfidenceScore int         `json:"confidenceScore,omitempty"`
		VoteEffect      *voteEffect `json:"voteEffect,omitempty"`
	}
	out := make([]itemPayload, 0, len(items))
	for _, it := range items {
		var q Question
		var left, right ModelAnswer
		var leftModel, rightModel Model
		a.db.First(&q, "id = ?", it.QuestionID)
		a.db.First(&left, "id = ?", it.LeftAnswerID)
		a.db.First(&right, "id = ?", it.RightAnswerID)
		a.db.First(&leftModel, "id = ?", left.ModelID)
		a.db.First(&rightModel, "id = ?", right.ModelID)
		var vote EvalVote
		voted := a.db.Where("user_id = ? AND item_id = ?", userID, it.ID).First(&vote).Error == nil
		payload := itemPayload{
			ItemID:   it.ID,
			Position: it.Position,
			Question: gin.H{"id": q.ID, "prompt": q.Prompt},
			Left:     gin.H{"answerId": left.ID, "text": left.AnswerText, "modelId": leftModel.ID, "modelName": leftModel.DisplayName},
			Right:    gin.H{"answerId": right.ID, "text": right.AnswerText, "modelId": rightModel.ID, "modelName": rightModel.DisplayName},
			Voted:    voted,
		}
		if voted {
			payload.ConfidenceScore = vote.ConfidenceScore
			outcome := strings.TrimSpace(vote.VoteOutcome)
			if outcome == "" {
				if vote.WinnerAnswerID == it.LeftAnswerID {
					outcome = "left"
				} else {
					outcome = "right"
				}
			}
			payload.Outcome = outcome
			if outcome == "left" {
				payload.WinnerSide = "left"
			} else if outcome == "right" {
				payload.WinnerSide = "right"
			}
			if strings.TrimSpace(vote.VoteEffectJSON) != "" {
				var effect voteEffect
				if err := json.Unmarshal([]byte(vote.VoteEffectJSON), &effect); err == nil {
					payload.VoteEffect = &effect
				}
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

func voteOutcomeLabel(outcome string) string {
	switch outcome {
	case "left":
		return "A 更好"
	case "right":
		return "B 更好"
	case "both_good":
		return "都好"
	case "both_bad":
		return "都不好"
	default:
		return "未知"
	}
}

type userHistoryRow struct {
	SessionID        string    `json:"sessionId"`
	SessionMode      string    `json:"sessionMode"`
	SessionCreatedAt time.Time `json:"sessionCreatedAt"`
	CategoryName     string    `json:"categoryName"`
	ItemID           string    `json:"itemId"`
	Position         int       `json:"position"`
	QuestionID       string    `json:"questionId"`
	QuestionPrompt   string    `json:"questionPrompt"`
	Outcome          string    `json:"outcome"`
	OutcomeLabel     string    `json:"outcomeLabel"`
	VotedAt          time.Time `json:"votedAt"`
	LeftModelID      string    `json:"leftModelId"`
	LeftModelName    string    `json:"leftModelName"`
	LeftAnswerID     string    `json:"leftAnswerId"`
	LeftAnswerText   string    `json:"leftAnswerText"`
	RightModelID     string    `json:"rightModelId"`
	RightModelName   string    `json:"rightModelName"`
	RightAnswerID    string    `json:"rightAnswerId"`
	RightAnswerText  string    `json:"rightAnswerText"`
	WinnerModelIDs   []string  `json:"winnerModelIds"`
	WinnerModels     []string  `json:"winnerModels"`
}

type userModelFitRow struct {
	Scope       string  `json:"scope"`
	ModelID     string  `json:"modelId"`
	ModelName   string  `json:"modelName"`
	Appearances int     `json:"appearances"`
	Positive    int     `json:"positive"`
	Rate        float64 `json:"rate"`
	Rank        int     `json:"rank"`
}

type userHistoryResponse struct {
	Items     []userHistoryRow  `json:"items"`
	Total     int64             `json:"total"`
	Page      int               `json:"page"`
	PageSize  int               `json:"pageSize"`
	FitScopes []string          `json:"fitScopes"`
	TopModels []userModelFitRow `json:"topModels"`
}

func fillHistoryWinners(rows []userHistoryRow) {
	for i := range rows {
		rows[i].OutcomeLabel = voteOutcomeLabel(rows[i].Outcome)
		switch rows[i].Outcome {
		case "left":
			rows[i].WinnerModelIDs = []string{rows[i].LeftModelID}
			rows[i].WinnerModels = []string{rows[i].LeftModelName}
		case "right":
			rows[i].WinnerModelIDs = []string{rows[i].RightModelID}
			rows[i].WinnerModels = []string{rows[i].RightModelName}
		case "both_good":
			rows[i].WinnerModelIDs = []string{rows[i].LeftModelID, rows[i].RightModelID}
			rows[i].WinnerModels = []string{rows[i].LeftModelName, rows[i].RightModelName}
		case "both_bad":
			rows[i].WinnerModelIDs = []string{}
			rows[i].WinnerModels = []string{}
		default:
			rows[i].WinnerModelIDs = []string{}
			rows[i].WinnerModels = []string{}
		}
	}
}

func buildUserModelFit(rows []userHistoryRow) ([]string, []userModelFitRow) {
	type modelAgg struct {
		modelID     string
		modelName   string
		appearances int
		positive    int
	}
	scopes := []string{"全部"}
	scopeSeen := map[string]bool{"全部": true}
	byScope := map[string]map[string]*modelAgg{}
	ensureAgg := func(scope, modelID, modelName string) *modelAgg {
		if byScope[scope] == nil {
			byScope[scope] = map[string]*modelAgg{}
		}
		if byScope[scope][modelID] == nil {
			byScope[scope][modelID] = &modelAgg{modelID: modelID, modelName: modelName}
		}
		return byScope[scope][modelID]
	}
	record := func(scope string, row userHistoryRow) {
		left := ensureAgg(scope, row.LeftModelID, row.LeftModelName)
		right := ensureAgg(scope, row.RightModelID, row.RightModelName)
		left.appearances++
		right.appearances++
		switch row.Outcome {
		case "left":
			left.positive++
		case "right":
			right.positive++
		case "both_good":
			left.positive++
			right.positive++
		}
	}
	for _, row := range rows {
		if !scopeSeen[row.CategoryName] {
			scopeSeen[row.CategoryName] = true
			scopes = append(scopes, row.CategoryName)
		}
		record("全部", row)
		record(row.CategoryName, row)
	}
	out := []userModelFitRow{}
	for _, scope := range scopes {
		models := make([]userModelFitRow, 0, len(byScope[scope]))
		for _, agg := range byScope[scope] {
			rate := 0.0
			if agg.appearances > 0 {
				rate = float64(agg.positive) / float64(agg.appearances)
			}
			models = append(models, userModelFitRow{
				Scope:       scope,
				ModelID:     agg.modelID,
				ModelName:   agg.modelName,
				Appearances: agg.appearances,
				Positive:    agg.positive,
				Rate:        rate,
			})
		}
		sort.Slice(models, func(i, j int) bool {
			if models[i].Positive != models[j].Positive {
				return models[i].Positive > models[j].Positive
			}
			if models[i].Rate != models[j].Rate {
				return models[i].Rate > models[j].Rate
			}
			if models[i].Appearances != models[j].Appearances {
				return models[i].Appearances > models[j].Appearances
			}
			return models[i].ModelName < models[j].ModelName
		})
		if len(models) > 3 {
			models = models[:3]
		}
		for i := range models {
			models[i].Rank = i + 1
		}
		out = append(out, models...)
	}
	return scopes, out
}

func (a *App) userHistory(c *gin.Context) {
	userID := currentUser(c).ID
	page, pageSize, offset := parseBrowsePagination(c)
	var total int64
	if err := a.db.Model(&EvalVote{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		fail(c, http.StatusInternalServerError, "读取历史评估失败")
		return
	}
	var rows []userHistoryRow
	if err := a.db.Raw(`
		SELECT
			s.id AS session_id,
			s.mode AS session_mode,
			s.created_at AS session_created_at,
			c.name AS category_name,
			i.id AS item_id,
			i.position AS position,
			q.id AS question_id,
			q.prompt AS question_prompt,
			CASE
				WHEN v.vote_outcome <> '' THEN v.vote_outcome
				WHEN v.winner_answer_id = i.left_answer_id THEN 'left'
				WHEN v.winner_answer_id = i.right_answer_id THEN 'right'
				ELSE ''
			END AS outcome,
			v.created_at AS voted_at,
			lm.id AS left_model_id,
			lm.display_name AS left_model_name,
			la.id AS left_answer_id,
			la.answer_text AS left_answer_text,
			rm.id AS right_model_id,
			rm.display_name AS right_model_name,
			ra.id AS right_answer_id,
			ra.answer_text AS right_answer_text
		FROM eval_votes v
		JOIN eval_items i ON i.id = v.item_id
		JOIN eval_sessions s ON s.id = v.session_id
		JOIN eval_categories c ON c.id = s.category_id
		JOIN questions q ON q.id = v.question_id
		JOIN model_answers la ON la.id = i.left_answer_id
		JOIN models lm ON lm.id = la.model_id
		JOIN model_answers ra ON ra.id = i.right_answer_id
		JOIN models rm ON rm.id = ra.model_id
		WHERE v.user_id = ?
		ORDER BY v.created_at DESC
		LIMIT ? OFFSET ?
	`, userID, pageSize, offset).Scan(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, "读取历史评估失败")
		return
	}
	if rows == nil {
		rows = []userHistoryRow{}
	}
	fillHistoryWinners(rows)
	var allRows []userHistoryRow
	if err := a.db.Raw(`
		SELECT
			s.id AS session_id,
			s.mode AS session_mode,
			s.created_at AS session_created_at,
			c.name AS category_name,
			i.id AS item_id,
			i.position AS position,
			q.id AS question_id,
			q.prompt AS question_prompt,
			CASE
				WHEN v.vote_outcome <> '' THEN v.vote_outcome
				WHEN v.winner_answer_id = i.left_answer_id THEN 'left'
				WHEN v.winner_answer_id = i.right_answer_id THEN 'right'
				ELSE ''
			END AS outcome,
			v.created_at AS voted_at,
			lm.id AS left_model_id,
			lm.display_name AS left_model_name,
			la.id AS left_answer_id,
			rm.id AS right_model_id,
			rm.display_name AS right_model_name,
			ra.id AS right_answer_id
		FROM eval_votes v
		JOIN eval_items i ON i.id = v.item_id
		JOIN eval_sessions s ON s.id = v.session_id
		JOIN eval_categories c ON c.id = s.category_id
		JOIN questions q ON q.id = v.question_id
		JOIN model_answers la ON la.id = i.left_answer_id
		JOIN models lm ON lm.id = la.model_id
		JOIN model_answers ra ON ra.id = i.right_answer_id
		JOIN models rm ON rm.id = ra.model_id
		WHERE v.user_id = ?
		ORDER BY v.created_at DESC
	`, userID).Scan(&allRows).Error; err != nil {
		fail(c, http.StatusInternalServerError, "读取模型适配统计失败")
		return
	}
	if allRows == nil {
		allRows = []userHistoryRow{}
	}
	fillHistoryWinners(allRows)
	scopes, topModels := buildUserModelFit(allRows)
	ok(c, userHistoryResponse{Items: rows, Total: total, Page: page, PageSize: pageSize, FitScopes: scopes, TopModels: topModels})
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

func parseEvalVoteOutcome(outcome, winnerSide string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(outcome)) {
	case "left", "a_better":
		return "left", nil
	case "right", "b_better":
		return "right", nil
	case "both_good":
		return "both_good", nil
	case "both_bad":
		return "both_bad", nil
	case "":
		switch strings.TrimSpace(strings.ToLower(winnerSide)) {
		case "left":
			return "left", nil
		case "right":
			return "right", nil
		default:
			return "", errors.New("缺少 outcome（left / right / both_good / both_bad），或兼容旧客户端时提供 winnerSide: left|right")
		}
	default:
		return "", errors.New("未知 outcome，允许: left, right, both_good, both_bad（或 a_better / b_better）")
	}
}

// 盲评四档：写入 eval_votes.confidence_score 的档位编码（1–5，兼容旧「信心分」列，仅作统计区分，不再由用户滑条决定）。
func voteTierStoredScore(outcome string) int {
	switch outcome {
	case "left", "right":
		return 5 // 单侧更好：等价于「强胜负」档
	case "both_good":
		return 3 // 平局·都好
	case "both_bad":
		return 2 // 平局·都不好（与 3 区分）
	default:
		return 3
	}
}

// eloKWin：A/B 更好时的胜负 Elo 调节强度（替代原 24+4*confidence）。
func eloKWin() float64 { return 40 }

// eloKDraw：平局公式 K；「都好」略强于「都不好」，二者均弱于明确胜负。
func eloKDraw(outcome string) float64 {
	if outcome == "both_bad" {
		return 14
	}
	return 28
}

func peerEloKWin() float64 { return 16 }

func peerEloKDraw() float64 { return 8 }

func modelRank(tx *gorm.DB, modelID, categoryID string) (int, error) {
	stat := getStat(tx, modelID, categoryID)
	var better int64
	if err := tx.Model(&ModelStat{}).Where("category_id = ? AND elo_rating > ?", categoryID, stat.EloRating).Count(&better).Error; err != nil {
		return 0, err
	}
	return int(better) + 1, nil
}

func modelEffectSnapshot(tx *gorm.DB, side, modelID, categoryID string) (voteModelEffect, error) {
	stat := getStat(tx, modelID, categoryID)
	rank, err := modelRank(tx, modelID, categoryID)
	if err != nil {
		return voteModelEffect{}, err
	}
	return voteModelEffect{Side: side, ModelID: modelID, EloBefore: stat.EloRating, EloAfter: stat.EloRating, RankBefore: rank, RankAfter: rank}, nil
}

func (a *App) applyVoteAndBuildEffect(tx *gorm.DB, item EvalItem, outcome string) (voteEffect, error) {
	var question Question
	var leftAnswer, rightAnswer ModelAnswer
	if err := tx.First(&question, "id = ?", item.QuestionID).Error; err != nil {
		return voteEffect{}, err
	}
	if err := tx.First(&leftAnswer, "id = ?", item.LeftAnswerID).Error; err != nil {
		return voteEffect{}, err
	}
	if err := tx.First(&rightAnswer, "id = ?", item.RightAnswerID).Error; err != nil {
		return voteEffect{}, err
	}
	leftEffect, err := modelEffectSnapshot(tx, "left", leftAnswer.ModelID, question.CategoryID)
	if err != nil {
		return voteEffect{}, err
	}
	rightEffect, err := modelEffectSnapshot(tx, "right", rightAnswer.ModelID, question.CategoryID)
	if err != nil {
		return voteEffect{}, err
	}
	switch outcome {
	case "left":
		err = a.updateEloWin(tx, item.QuestionID, item.LeftAnswerID, item.RightAnswerID, eloKWin())
	case "right":
		err = a.updateEloWin(tx, item.QuestionID, item.RightAnswerID, item.LeftAnswerID, eloKWin())
	case "both_good", "both_bad":
		err = a.updateEloDraw(tx, item.QuestionID, item.LeftAnswerID, item.RightAnswerID, eloKDraw(outcome))
	default:
		err = errors.New("内部错误：未知 outcome")
	}
	if err != nil {
		return voteEffect{}, err
	}
	for _, target := range []*voteModelEffect{&leftEffect, &rightEffect} {
		stat := getStat(tx, target.ModelID, question.CategoryID)
		rank, err := modelRank(tx, target.ModelID, question.CategoryID)
		if err != nil {
			return voteEffect{}, err
		}
		target.EloAfter = stat.EloRating
		target.EloDelta = stat.EloRating - target.EloBefore
		target.RankAfter = rank
		target.RankDelta = target.RankBefore - rank
	}
	return voteEffect{CategoryID: question.CategoryID, Models: []voteModelEffect{leftEffect, rightEffect}}, nil
}

func (a *App) vote(c *gin.Context) {
	var req struct {
		ItemID     string `json:"itemId"`
		Outcome    string `json:"outcome"`
		WinnerSide string `json:"winnerSide"`
	}
	if !bind(c, &req) {
		return
	}
	outcome, err := parseEvalVoteOutcome(req.Outcome, req.WinnerSide)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	storedScore := voteTierStoredScore(outcome)
	var item EvalItem
	if err := a.db.First(&item, "id = ?", req.ItemID).Error; err != nil {
		fail(c, http.StatusNotFound, "盲评题不存在")
		return
	}
	winnerAnswerID := item.LeftAnswerID
	switch outcome {
	case "left":
		winnerAnswerID = item.LeftAnswerID
	case "right":
		winnerAnswerID = item.RightAnswerID
	case "both_good", "both_bad":
		// 满足非空约束；真实语义由 vote_outcome 表示
		winnerAnswerID = item.LeftAnswerID
	}
	userID := currentUser(c).ID
	vote := EvalVote{
		ID:              newID(),
		UserID:          userID,
		SessionID:       item.SessionID,
		ItemID:          item.ID,
		QuestionID:      item.QuestionID,
		WinnerAnswerID:  winnerAnswerID,
		VoteOutcome:     outcome,
		ConfidenceScore: storedScore,
		RatingScale:     5,
		CreatedAt:       time.Now(),
	}
	var effect voteEffect
	err = a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&vote).Error; err != nil {
			return err
		}
		nextEffect, err := a.applyVoteAndBuildEffect(tx, item, outcome)
		if err != nil {
			return err
		}
		effect = nextEffect
		encodedEffect, err := json.Marshal(effect)
		if err != nil {
			return err
		}
		vote.VoteEffectJSON = string(encodedEffect)
		return tx.Model(&EvalVote{}).Where("id = ?", vote.ID).Update("vote_effect_json", vote.VoteEffectJSON).Error
	})
	if err != nil {
		fail(c, http.StatusConflict, "该题已投票或统计失败")
		return
	}
	ok(c, gin.H{"vote": vote, "effect": effect})
}

func (a *App) updateEloWin(tx *gorm.DB, questionID, winnerAnswerID, loserAnswerID string, k float64) error {
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
	return updateEloWinByModels(tx, question.CategoryID, winnerAnswer.ModelID, loserAnswer.ModelID, k)
}

func (a *App) updateElo(tx *gorm.DB, questionID, winnerAnswerID, loserAnswerID string, confidence int) error {
	k := 24 + float64(confidence)*4
	return a.updateEloWin(tx, questionID, winnerAnswerID, loserAnswerID, k)
}

func updateEloWinByModels(tx *gorm.DB, categoryID, winnerModelID, loserModelID string, k float64) error {
	winnerStat := getStat(tx, winnerModelID, categoryID)
	loserStat := getStat(tx, loserModelID, categoryID)
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

// updateEloDraw：平局（都好 / 都不好），双方各记 0.5 分，对称调整 Elo；双方都计入 vote 与 draw。
func (a *App) updateEloDraw(tx *gorm.DB, questionID, leftAnswerID, rightAnswerID string, k float64) error {
	var question Question
	var leftAnswer, rightAnswer ModelAnswer
	if err := tx.First(&question, "id = ?", questionID).Error; err != nil {
		return err
	}
	if err := tx.First(&leftAnswer, "id = ?", leftAnswerID).Error; err != nil {
		return err
	}
	if err := tx.First(&rightAnswer, "id = ?", rightAnswerID).Error; err != nil {
		return err
	}
	return updateEloDrawByModels(tx, question.CategoryID, leftAnswer.ModelID, rightAnswer.ModelID, k)
}

func updateEloDrawByModels(tx *gorm.DB, categoryID, leftModelID, rightModelID string, k float64) error {
	leftStat := getStat(tx, leftModelID, categoryID)
	rightStat := getStat(tx, rightModelID, categoryID)
	// 左方期望得分 E_left（对右）
	eLeft := 1 / (1 + math.Pow(10, (rightStat.EloRating-leftStat.EloRating)/400))
	deltaLeft := k * (0.5 - eLeft)
	deltaRight := -deltaLeft

	leftStat.EloRating += deltaLeft
	leftStat.LastEloDelta = deltaLeft
	leftStat.VoteCount++
	leftStat.DrawCount++
	leftStat.UpdatedAt = time.Now()

	rightStat.EloRating += deltaRight
	rightStat.LastEloDelta = deltaRight
	rightStat.VoteCount++
	rightStat.DrawCount++
	rightStat.UpdatedAt = time.Now()

	if err := tx.Save(&leftStat).Error; err != nil {
		return err
	}
	return tx.Save(&rightStat).Error
}

func getStat(tx *gorm.DB, modelID, categoryID string) ModelStat {
	var stat ModelStat
	if err := tx.First(&stat, "model_id = ? AND category_id = ?", modelID, categoryID).Error; err != nil {
		stat = ModelStat{ModelID: modelID, CategoryID: categoryID, EloRating: defaultElo, UpdatedAt: time.Now()}
		tx.Create(&stat)
	}
	return stat
}

func applyModelPeerVote(db *gorm.DB, voteID string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var vote ModelPeerVote
		if err := tx.First(&vote, "id = ?", voteID).Error; err != nil {
			return err
		}
		if vote.Applied {
			return nil
		}
		leftEffect, err := modelEffectSnapshot(tx, "left", vote.LeftModelID, vote.CategoryID)
		if err != nil {
			return err
		}
		rightEffect, err := modelEffectSnapshot(tx, "right", vote.RightModelID, vote.CategoryID)
		if err != nil {
			return err
		}
		switch vote.Outcome {
		case "left":
			err = updateEloWinByModels(tx, vote.CategoryID, vote.LeftModelID, vote.RightModelID, peerEloKWin())
		case "right":
			err = updateEloWinByModels(tx, vote.CategoryID, vote.RightModelID, vote.LeftModelID, peerEloKWin())
		case "both_good", "both_bad":
			err = updateEloDrawByModels(tx, vote.CategoryID, vote.LeftModelID, vote.RightModelID, peerEloKDraw())
		default:
			err = fmt.Errorf("unknown peer vote outcome %s", vote.Outcome)
		}
		if err != nil {
			return err
		}
		for _, target := range []*voteModelEffect{&leftEffect, &rightEffect} {
			stat := getStat(tx, target.ModelID, vote.CategoryID)
			rank, err := modelRank(tx, target.ModelID, vote.CategoryID)
			if err != nil {
				return err
			}
			target.EloAfter = stat.EloRating
			target.EloDelta = stat.EloRating - target.EloBefore
			target.RankAfter = rank
			target.RankDelta = target.RankBefore - rank
		}
		effect := voteEffect{CategoryID: vote.CategoryID, Models: []voteModelEffect{leftEffect, rightEffect}}
		effectJSON, err := json.Marshal(effect)
		if err != nil {
			return err
		}
		return tx.Model(&ModelPeerVote{}).Where("id = ?", vote.ID).Updates(map[string]any{
			"applied":     true,
			"effect_json": string(effectJSON),
		}).Error
	})
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

func (a *App) rankingPeerMatrix(c *gin.Context) {
	categoryID := c.Query("categoryId")
	type modelRow struct {
		ModelID     string `json:"modelId"`
		DisplayName string `json:"displayName"`
		Provider    string `json:"provider"`
	}
	type cellAgg struct {
		JudgeModelID  string  `json:"judgeModelId"`
		TargetModelID string  `json:"targetModelId"`
		Score         float64 `json:"score"`
		Samples       int64   `json:"samples"`
		Positive      int64   `json:"positive"`
		Negative      int64   `json:"negative"`
		BothGood      int64   `json:"bothGood"`
		BothBad       int64   `json:"bothBad"`
	}
	type matrixResponse struct {
		Models      []modelRow `json:"models"`
		Cells       []cellAgg  `json:"cells"`
		SampleCount int64      `json:"sampleCount"`
	}

	var models []Model
	if err := a.db.Where("enabled = ?", true).Order("display_name asc").Find(&models).Error; err != nil {
		fail(c, http.StatusInternalServerError, "读取模型失败")
		return
	}
	outModels := make([]modelRow, 0, len(models))
	for _, model := range models {
		outModels = append(outModels, modelRow{ModelID: model.ID, DisplayName: model.DisplayName, Provider: model.Provider})
	}

	var votes []ModelPeerVote
	query := a.db.Where("applied = ?", true)
	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}
	if err := query.Find(&votes).Error; err != nil {
		fail(c, http.StatusInternalServerError, "读取模型互评失败")
		return
	}

	cells := map[string]*cellAgg{}
	add := func(judgeID, targetID string, score float64, outcome string) {
		key := judgeID + ":" + targetID
		cell := cells[key]
		if cell == nil {
			cell = &cellAgg{JudgeModelID: judgeID, TargetModelID: targetID}
			cells[key] = cell
		}
		cell.Score += score
		cell.Samples++
		switch outcome {
		case "left", "right":
			if score > 0 {
				cell.Positive++
			} else if score < 0 {
				cell.Negative++
			}
		case "both_good":
			cell.BothGood++
		case "both_bad":
			cell.BothBad++
		}
	}
	for _, vote := range votes {
		add(vote.JudgeModelID, vote.LeftModelID, peerTargetScore(vote.Outcome, true), vote.Outcome)
		add(vote.JudgeModelID, vote.RightModelID, peerTargetScore(vote.Outcome, false), vote.Outcome)
	}
	outCells := make([]cellAgg, 0, len(cells))
	for _, cell := range cells {
		if cell.Samples > 0 {
			cell.Score = cell.Score / float64(cell.Samples)
		}
		outCells = append(outCells, *cell)
	}
	sort.Slice(outCells, func(i, j int) bool {
		if outCells[i].JudgeModelID == outCells[j].JudgeModelID {
			return outCells[i].TargetModelID < outCells[j].TargetModelID
		}
		return outCells[i].JudgeModelID < outCells[j].JudgeModelID
	})
	ok(c, matrixResponse{Models: outModels, Cells: outCells, SampleCount: int64(len(votes))})
}

func (a *App) cumulativeTrend(model any, days int) ([]dashboardTrendPoint, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	points := make([]dashboardTrendPoint, 0, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		end := day.Add(24*time.Hour - time.Nanosecond)
		var count int64
		if err := a.db.Model(model).Where("created_at <= ?", end).Count(&count).Error; err != nil {
			return nil, err
		}
		points = append(points, dashboardTrendPoint{Date: day.Format("01-02"), Count: count})
	}
	return points, nil
}

func flatTrend(days int, count int64) []dashboardTrendPoint {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	points := make([]dashboardTrendPoint, 0, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		points = append(points, dashboardTrendPoint{Date: day.Format("01-02"), Count: count})
	}
	return points
}

func (a *App) dashboard(c *gin.Context) {
	var users, votes, questions, models int64
	a.db.Model(&User{}).Count(&users)
	a.db.Model(&EvalVote{}).Count(&votes)
	a.db.Model(&Question{}).Count(&questions)
	a.db.Model(&Model{}).Where("enabled = ?", true).Count(&models)
	userTrend, err := a.cumulativeTrend(&User{}, 14)
	if err != nil {
		fail(c, http.StatusInternalServerError, "读取用户趋势失败")
		return
	}
	voteTrend, err := a.cumulativeTrend(&EvalVote{}, 14)
	if err != nil {
		fail(c, http.StatusInternalServerError, "读取投票趋势失败")
		return
	}
	questionTrend, err := a.cumulativeTrend(&Question{}, 14)
	if err != nil {
		fail(c, http.StatusInternalServerError, "读取题目趋势失败")
		return
	}
	var categories []EvalCategory
	a.db.Where("enabled = ?", true).Order("sort_order asc").Find(&categories)
	domainBySlug := map[string]evalDomainMeta{}
	for _, domain := range loadEvalDomainMetas() {
		domainBySlug[domain.Slug] = domain
	}
	for i := range categories {
		slug := domainSlugForCategory(categories[i])
		if domain, ok := domainBySlug[slug]; ok {
			if strings.TrimSpace(domain.Name) != "" {
				categories[i].Name = domain.Name
			}
			if strings.TrimSpace(domain.Description) != "" {
				categories[i].Description = domain.Description
			}
		}
	}
	var top []map[string]any
	a.db.Raw(`SELECT m.display_name, ms.elo_rating, ms.vote_count FROM model_stats ms JOIN models m ON m.id = ms.model_id ORDER BY ms.elo_rating DESC LIMIT 5`).Scan(&top)
	ok(c, gin.H{
		"users":     users,
		"votes":     votes,
		"questions": questions,
		"models":    models,
		"trends": gin.H{
			"users":     userTrend,
			"votes":     voteTrend,
			"questions": questionTrend,
			"models":    flatTrend(14, models),
		},
		"categories": categories,
		"topModels":  top,
	})
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
