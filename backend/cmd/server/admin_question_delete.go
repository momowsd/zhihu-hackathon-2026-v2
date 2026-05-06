package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxQuestionDeleteBatch = 100

type questionDeleteImpactOut struct {
	Votes        int64 `json:"votes"`
	EvalItems    int64 `json:"evalItems"`
	ModelAnswers int64 `json:"modelAnswers"`
	Questions    int64 `json:"questions"`
}

func (a *App) adminQuestionDeleteImpactSingle(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		fail(c, http.StatusBadRequest, "缺少题目 id")
		return
	}
	out, err := a.computeQuestionDeleteImpact(a.db, []string{id})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, out)
}

func (a *App) adminQuestionDeleteImpactBatch(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if !bind(c, &req) {
		return
	}
	if len(req.IDs) == 0 {
		fail(c, http.StatusBadRequest, "ids 不能为空")
		return
	}
	if len(req.IDs) > maxQuestionDeleteBatch {
		fail(c, http.StatusBadRequest, "一次最多删除 100 条题目")
		return
	}
	out, err := a.computeQuestionDeleteImpact(a.db, req.IDs)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, out)
}

func (a *App) computeQuestionDeleteImpact(db *gorm.DB, questionIDs []string) (questionDeleteImpactOut, error) {
	var out questionDeleteImpactOut
	if len(questionIDs) == 0 {
		return out, nil
	}
	if err := db.Model(&Question{}).Where("id IN ?", questionIDs).Count(&out.Questions).Error; err != nil {
		return out, err
	}
	if err := db.Model(&EvalItem{}).Where("question_id IN ?", questionIDs).Count(&out.EvalItems).Error; err != nil {
		return out, err
	}
	if err := db.Model(&ModelAnswer{}).Where("question_id IN ?", questionIDs).Count(&out.ModelAnswers).Error; err != nil {
		return out, err
	}
	var itemIDs []string
	if err := db.Model(&EvalItem{}).Where("question_id IN ?", questionIDs).Pluck("id", &itemIDs).Error; err != nil {
		return out, err
	}
	q := db.Model(&EvalVote{}).Where("question_id IN ?", questionIDs)
	if len(itemIDs) > 0 {
		q = q.Or("item_id IN ?", itemIDs)
	}
	if err := q.Count(&out.Votes).Error; err != nil {
		return out, err
	}
	return out, nil
}

func (a *App) deleteQuestionsCascade(tx *gorm.DB, questionIDs []string) error {
	if len(questionIDs) == 0 {
		return nil
	}
	var itemIDs []string
	if err := tx.Model(&EvalItem{}).Where("question_id IN ?", questionIDs).Pluck("id", &itemIDs).Error; err != nil {
		return err
	}
	qv := tx.Where("question_id IN ?", questionIDs)
	if len(itemIDs) > 0 {
		qv = qv.Or("item_id IN ?", itemIDs)
	}
	if err := qv.Delete(&EvalVote{}).Error; err != nil {
		return err
	}
	if err := tx.Where("question_id IN ?", questionIDs).Delete(&EvalItem{}).Error; err != nil {
		return err
	}
	if err := tx.Where("question_id IN ?", questionIDs).Delete(&ModelAnswer{}).Error; err != nil {
		return err
	}
	if err := tx.Where("id IN ?", questionIDs).Delete(&Question{}).Error; err != nil {
		return err
	}
	return nil
}

func (a *App) adminDeleteQuestion(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		fail(c, http.StatusBadRequest, "缺少题目 id")
		return
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		return a.deleteQuestionsCascade(tx, []string{id})
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"deleted": 1})
}

func (a *App) adminDeleteQuestionsBatch(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if !bind(c, &req) {
		return
	}
	if len(req.IDs) == 0 {
		fail(c, http.StatusBadRequest, "ids 不能为空")
		return
	}
	if len(req.IDs) > maxQuestionDeleteBatch {
		fail(c, http.StatusBadRequest, "一次最多删除 100 条题目")
		return
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		return a.deleteQuestionsCascade(tx, req.IDs)
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"deleted": len(req.IDs)})
}
