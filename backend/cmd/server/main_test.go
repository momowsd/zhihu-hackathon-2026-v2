package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateAndSeed(db); err != nil {
		t.Fatal(err)
	}
	return &App{db: db, cfg: Config{JWTSecret: "test", HTTPTimeout: time.Second}, client: http.DefaultClient}
}

func TestUpdateEloRecordsWinAndLoss(t *testing.T) {
	app := newTestApp(t)
	var question Question
	if err := app.db.First(&question).Error; err != nil {
		t.Fatal(err)
	}
	var answers []ModelAnswer
	if err := app.db.Where("question_id = ?", question.ID).Limit(2).Find(&answers).Error; err != nil {
		t.Fatal(err)
	}
	if len(answers) != 2 {
		t.Fatalf("expected two answers, got %d", len(answers))
	}
	if err := app.updateElo(app.db, question.ID, answers[0].ID, answers[1].ID, 5); err != nil {
		t.Fatal(err)
	}
	var winner, loser ModelStat
	if err := app.db.First(&winner, "model_id = ? AND category_id = ?", answers[0].ModelID, question.CategoryID).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.db.First(&loser, "model_id = ? AND category_id = ?", answers[1].ModelID, question.CategoryID).Error; err != nil {
		t.Fatal(err)
	}
	if winner.WinCount != 1 || loser.LossCount != 1 {
		t.Fatalf("unexpected counts: winner=%+v loser=%+v", winner, loser)
	}
	if winner.EloRating <= defaultElo || loser.EloRating >= defaultElo {
		t.Fatalf("elo not updated: winner=%f loser=%f", winner.EloRating, loser.EloRating)
	}
}

func TestHealthz(t *testing.T) {
	app := newTestApp(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	app.router().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
}
