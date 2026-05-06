package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	importBundleVersion  = 1
	maxImportCategories    = 500
	maxImportModels        = 500
	maxImportQuestions     = 5000
	maxImportAnswers       = 20000
	importBundleErrorCap   = 80
)

type importBundle struct {
	Version    int              `json:"version"`
	Categories []bundleCategory `json:"categories"`
	Models     []bundleModel    `json:"models"`
	Questions  []bundleQuestion `json:"questions"`
	Answers    []bundleAnswer   `json:"answers"`
}

type bundleCategory struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sortOrder"`
}

type bundleModel struct {
	Ref         string `json:"ref"`
	Provider    string `json:"provider"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Version     string `json:"version"`
	IsBaseline  bool   `json:"isBaseline"`
	Enabled     bool   `json:"enabled"`
}

type bundleQuestion struct {
	Ref          string `json:"ref"`
	CategoryCode string `json:"categoryCode"`
	Prompt       string `json:"prompt"`
	Source       string `json:"source"`
	Difficulty   string `json:"difficulty"`
	Enabled      bool   `json:"enabled"`
}

type bundleAnswer struct {
	QuestionRef  string `json:"questionRef"`
	ModelRef     string `json:"modelRef"`
	AnswerText   string `json:"answerText"`
	MetadataJSON string `json:"metadataJson"`
}

type importFieldError struct {
	Row     string `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (a *App) adminImportBundle(c *gin.Context) {
	var raw importBundle
	if err := c.ShouldBindJSON(&raw); err != nil {
		fail(c, http.StatusBadRequest, "请求体须为 JSON")
		return
	}
	errs, err := a.runImportBundle(&raw)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if len(errs) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    http.StatusBadRequest,
			"message": "导入校验失败，请根据 errors 逐条修正后重试",
			"errors":  errs,
		})
		return
	}
	ok(c, gin.H{"ok": true})
}

func (a *App) runImportBundle(b *importBundle) ([]importFieldError, error) {
	errs := prevalidateImportBundle(a.db, b)
	if len(errs) > 0 {
		return errs, nil
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		return executeImportBundle(tx, b)
	})
	return nil, err
}

func prevalidateImportBundle(db *gorm.DB, b *importBundle) []importFieldError {
	var errs []importFieldError
	add := func(row, field, msg string) {
		if len(errs) >= importBundleErrorCap {
			return
		}
		errs = append(errs, importFieldError{Row: row, Field: field, Message: msg})
	}

	if b.Version != importBundleVersion {
		add("bundle", "version", fmt.Sprintf("仅支持 version=%d", importBundleVersion))
		return errs
	}
	if len(b.Categories) > maxImportCategories {
		add("bundle", "categories", fmt.Sprintf("分类数量超过上限 %d", maxImportCategories))
	}
	if len(b.Models) > maxImportModels {
		add("bundle", "models", fmt.Sprintf("模型数量超过上限 %d", maxImportModels))
	}
	if len(b.Questions) > maxImportQuestions {
		add("bundle", "questions", fmt.Sprintf("题目数量超过上限 %d", maxImportQuestions))
	}
	if len(b.Answers) > maxImportAnswers {
		add("bundle", "answers", fmt.Sprintf("回答数量超过上限 %d", maxImportAnswers))
	}
	if len(errs) > 0 {
		return errs
	}

	codesFromBundle := map[string]struct{}{}
	for i, cat := range b.Categories {
		row := fmt.Sprintf("categories[%d]", i)
		code := strings.TrimSpace(cat.Code)
		name := strings.TrimSpace(cat.Name)
		if code == "" {
			add(row, "code", "code 不能为空")
			continue
		}
		if name == "" {
			add(row, "name", "name 不能为空")
			continue
		}
		codesFromBundle[code] = struct{}{}
	}

	seenQRef := map[string]struct{}{}
	for i, q := range b.Questions {
		row := fmt.Sprintf("questions[%d]", i)
		ref := strings.TrimSpace(q.Ref)
		if ref == "" {
			add(row, "ref", "ref 不能为空")
			continue
		}
		if _, ok := seenQRef[ref]; ok {
			add(row, "ref", "ref 重复："+ref)
			continue
		}
		cc := strings.TrimSpace(q.CategoryCode)
		if cc == "" {
			add(row, "categoryCode", "categoryCode 不能为空")
			continue
		}
		if _, ok := codesFromBundle[cc]; !ok {
			var n int64
			db.Model(&EvalCategory{}).Where("code = ?", cc).Count(&n)
			if n == 0 {
				add(row, "categoryCode", "分类不存在（请在 bundle.categories 中声明或确保库里已有该 code）："+cc)
				continue
			}
		}
		seenQRef[ref] = struct{}{}
	}

	seenMRef := map[string]struct{}{}
	for i, m := range b.Models {
		row := fmt.Sprintf("models[%d]", i)
		ref := strings.TrimSpace(m.Ref)
		if ref == "" {
			add(row, "ref", "ref 不能为空")
			continue
		}
		if _, ok := seenMRef[ref]; ok {
			add(row, "ref", "ref 重复："+ref)
			continue
		}
		prov := strings.TrimSpace(m.Provider)
		nm := strings.TrimSpace(m.Name)
		dn := strings.TrimSpace(m.DisplayName)
		if prov == "" || nm == "" || dn == "" {
			add(row, "provider", "provider、name、displayName 均不能为空")
			continue
		}
		seenMRef[ref] = struct{}{}
	}

	for i, q := range b.Questions {
		row := fmt.Sprintf("questions[%d]", i)
		if strings.TrimSpace(q.Prompt) == "" {
			add(row, "prompt", "prompt 不能为空")
		}
	}

	for i, an := range b.Answers {
		row := fmt.Sprintf("answers[%d]", i)
		qref := strings.TrimSpace(an.QuestionRef)
		mref := strings.TrimSpace(an.ModelRef)
		if strings.TrimSpace(an.AnswerText) == "" {
			add(row, "answerText", "answerText 不能为空")
		}
		if qref == "" {
			add(row, "questionRef", "questionRef 不能为空")
		} else if _, ok := seenQRef[qref]; !ok {
			add(row, "questionRef", "未定义的 questionRef："+qref)
		}
		if mref == "" {
			add(row, "modelRef", "modelRef 不能为空")
		} else if _, ok := seenMRef[mref]; !ok {
			add(row, "modelRef", "未定义的 modelRef："+mref)
		}
		meta := strings.TrimSpace(an.MetadataJSON)
		if meta == "" {
			meta = "{}"
		}
		if !json.Valid([]byte(meta)) {
			add(row, "metadataJson", "须为合法 JSON 字符串")
		}
	}

	return errs
}

func executeImportBundle(tx *gorm.DB, b *importBundle) error {
	categoryCodeToID := map[string]string{}

	for _, cat := range b.Categories {
		code := strings.TrimSpace(cat.Code)
		name := strings.TrimSpace(cat.Name)
		var existing EvalCategory
		err := tx.Where("code = ?", code).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row := EvalCategory{
				ID:          newID(),
				Code:        code,
				Name:        name,
				Description: strings.TrimSpace(cat.Description),
				Enabled:     cat.Enabled,
				SortOrder:   cat.SortOrder,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			categoryCodeToID[code] = row.ID
			continue
		}
		if err != nil {
			return err
		}
		updates := map[string]any{
			"name":        name,
			"description": strings.TrimSpace(cat.Description),
			"enabled":     cat.Enabled,
			"sort_order":  cat.SortOrder,
		}
		if err := tx.Model(&EvalCategory{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return err
		}
		categoryCodeToID[code] = existing.ID
	}

	var dbCats []EvalCategory
	if err := tx.Find(&dbCats).Error; err != nil {
		return err
	}
	for _, c := range dbCats {
		if _, ok := categoryCodeToID[c.Code]; !ok {
			categoryCodeToID[c.Code] = c.ID
		}
	}

	modelRefToID := map[string]string{}
	for _, m := range b.Models {
		ref := strings.TrimSpace(m.Ref)
		prov := strings.TrimSpace(m.Provider)
		nm := strings.TrimSpace(m.Name)
		dn := strings.TrimSpace(m.DisplayName)
		var found Model
		err := tx.Where("provider = ? AND name = ?", prov, nm).First(&found).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			mo := Model{
				ID:          newID(),
				Provider:    prov,
				Name:        nm,
				DisplayName: dn,
				Version:     strings.TrimSpace(m.Version),
				IsBaseline:  m.IsBaseline,
				Enabled:     m.Enabled,
			}
			if err := tx.Create(&mo).Error; err != nil {
				return err
			}
			modelRefToID[ref] = mo.ID
		} else if err != nil {
			return err
		} else {
			modelRefToID[ref] = found.ID
		}
	}

	questionRefToID := map[string]string{}
	now := time.Now()
	for _, q := range b.Questions {
		ref := strings.TrimSpace(q.Ref)
		cc := strings.TrimSpace(q.CategoryCode)
		cid := categoryCodeToID[cc]
		src := strings.TrimSpace(q.Source)
		if src == "" {
			src = "import"
		}
		diff := strings.TrimSpace(q.Difficulty)
		if diff == "" {
			diff = "normal"
		}
		qr := Question{
			ID:         newID(),
			CategoryID: cid,
			Prompt:     strings.TrimSpace(q.Prompt),
			Source:     src,
			Difficulty: diff,
			Enabled:    q.Enabled,
			CreatedAt:  now,
		}
		if err := tx.Create(&qr).Error; err != nil {
			return err
		}
		questionRefToID[ref] = qr.ID
	}

	for _, an := range b.Answers {
		qid := questionRefToID[strings.TrimSpace(an.QuestionRef)]
		mid := modelRefToID[strings.TrimSpace(an.ModelRef)]
		meta := strings.TrimSpace(an.MetadataJSON)
		if meta == "" {
			meta = "{}"
		}
		ar := ModelAnswer{
			ID:           newID(),
			QuestionID:   qid,
			ModelID:      mid,
			AnswerText:   strings.TrimSpace(an.AnswerText),
			MetadataJSON: meta,
			CreatedAt:    now,
		}
		if err := tx.Create(&ar).Error; err != nil {
			return err
		}
	}
	return nil
}
