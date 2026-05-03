package main

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func parseBrowsePagination(c *gin.Context) (page, pageSize, offset int) {
	page = 1
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	pageSize = 20
	if v := c.Query("pageSize"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pageSize = n
			if pageSize > 200 {
				pageSize = 200
			}
		}
	}
	offset = (page - 1) * pageSize
	return page, pageSize, offset
}

// adminBrowseTable returns paginated rows for a whitelisted SQLite-backed table (admin only).
func (a *App) adminBrowseTable(c *gin.Context) {
	table := c.Param("table")
	page, pageSize, offset := parseBrowsePagination(c)

	var total int64
	var items any
	db := a.db

	switch table {
	case "users":
		var rows []User
		if err := db.Model(&User{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	case "eval_categories":
		var rows []EvalCategory
		if err := db.Model(&EvalCategory{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("sort_order asc, id asc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	case "questions":
		var rows []Question
		if err := db.Model(&Question{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	case "models":
		var rows []Model
		if err := db.Model(&Model{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("provider asc, display_name asc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	case "model_answers":
		var rows []ModelAnswer
		if err := db.Model(&ModelAnswer{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	case "eval_sessions":
		var rows []EvalSession
		if err := db.Model(&EvalSession{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	case "eval_items":
		var rows []EvalItem
		if err := db.Model(&EvalItem{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	case "eval_votes":
		var rows []EvalVote
		if err := db.Model(&EvalVote{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	case "model_stats":
		var rows []ModelStat
		if err := db.Model(&ModelStat{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("category_id asc, model_id asc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	case "submitted_endpoints":
		var rows []SubmittedEndpoint
		if err := db.Model(&SubmittedEndpoint{}).Count(&total).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if err := db.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		items = rows
	default:
		fail(c, http.StatusNotFound, "未知数据表")
		return
	}

	ok(c, gin.H{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}
