package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func testHandler(db *sql.DB) http.Handler {
	return newHandler(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func request(h http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestValidationAndRoutes(t *testing.T) {
	h := testHandler(nil)
	for _, tc := range []struct {
		name, method, path, body, contentType string
		status, allow                         int
	}{
		{"missing content type", "POST", "/tasks", `{"title":"a"}`, "", 400, 0},
		{"unknown field", "POST", "/tasks", `{"title":"a","extra":1}`, "application/json", 400, 0},
		{"duplicate title", "POST", "/tasks", `{"title":"a","title":"b"}`, "application/json", 400, 0},
		{"incorrect key case", "POST", "/tasks", `{"Title":"a"}`, "application/json", 400, 0},
		{"missing title", "POST", "/tasks", `{}`, "application/json", 400, 0},
		{"nonstring title", "POST", "/tasks", `{"title":1}`, "application/json", 400, 0},
		{"trailing json", "POST", "/tasks", `{"title":"a"}{}`, "application/json", 400, 0},
		{"blank title", "POST", "/tasks", `{"title":" \t"}`, "application/json", 400, 0},
		{"long title", "POST", "/tasks", `{"title":"` + strings.Repeat("é", 121) + `"}`, "application/json", 400, 0},
		{"NUL", "POST", "/tasks", `{"title":"\u0000"}`, "application/json", 400, 0},
		{"large body", "POST", "/tasks", `{"title":"` + strings.Repeat("a", 4100) + `"}`, "application/json", 400, 0},
		{"bad charset", "POST", "/tasks", `{"title":"a"}`, "application/json; charset=iso-8859-1", 400, 0},
		{"invalid uuid", "DELETE", "/tasks/not-a-uuid", "", "", 400, 0},
		{"unknown route", "GET", "/none", "", "", 404, 0},
		{"wrong method", "PUT", "/tasks", "", "", 405, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(h, tc.method, tc.path, tc.body, tc.contentType)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if tc.allow == 1 && w.Header().Get("Allow") == "" {
				t.Fatal("missing Allow header")
			}
			var got map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got) != 2 || got["error"] == nil || got["message"] == nil {
				t.Fatalf("invalid error response: %s", w.Body.String())
			}
		})
	}
	if w := request(h, "GET", "/healthz", "", ""); w.Code != 200 || w.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("healthz: %d %s", w.Code, w.Body.String())
	}
	if w := request(h, "GET", "/metrics", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `route="/tasks"`) {
		t.Fatalf("metrics: %d %s", w.Code, w.Body.String())
	}
}

func TestUUIDAndTitle(t *testing.T) {
	for range 20 {
		id, err := newUUID()
		if err != nil || !validUUID(id) || id[14] != '4' || !strings.ContainsAny(string(id[19]), "89ab") {
			t.Fatalf("invalid generated UUID: %q %v", id, err)
		}
	}
	if !validUUID("550E8400-E29B-41D4-A716-446655440000") || validUUID("550e8400-e29b-41d4-a716-44665544000z") {
		t.Fatal("canonical UUID validation failed")
	}
	if got, ok := normalizeTitle("\u2003é界\u2003"); !ok || got != "é界" {
		t.Fatalf("title normalization: %q %v", got, ok)
	}
}

func TestDatabaseIntegration(t *testing.T) {
	for _, key := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "POSTGRES_USER", "POSTGRES_PASSWORD"} {
		if os.Getenv(key) == "" {
			t.Skip("live PostgreSQL configuration not provided")
		}
	}
	cfg, err := readConfig()
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", cfg.databaseURL())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skip("live PostgreSQL unavailable")
	}
	if err := initialize(db); err != nil {
		t.Fatal(err)
	}
	h := testHandler(db)
	created := request(h, "POST", "/tasks", `{"title":"  integration task  "}`, "application/json; charset=utf-8")
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var task task
	if err := json.Unmarshal(created.Body.Bytes(), &task); err != nil || task.Title != "integration task" || !validUUID(task.ID) || task.CreatedAt == "" {
		t.Fatalf("created task: %+v %v", task, err)
	}
	t.Cleanup(func() { request(h, "DELETE", "/tasks/"+task.ID, "", "") })
	listed := request(h, "GET", "/tasks", "", "")
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), task.ID) {
		t.Fatalf("list: %d %s", listed.Code, listed.Body.String())
	}
	if deleted := request(h, "DELETE", "/tasks/"+strings.ToUpper(task.ID), "", ""); deleted.Code != 204 || deleted.Body.Len() != 0 {
		t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
	}
	if missing := request(h, "DELETE", "/tasks/"+task.ID, "", ""); missing.Code != 404 {
		t.Fatalf("missing delete: %d %s", missing.Code, missing.Body.String())
	}
}
