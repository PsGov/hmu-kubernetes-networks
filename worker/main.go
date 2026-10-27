package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const schema = `CREATE TABLE IF NOT EXISTS tasks (
    id UUID PRIMARY KEY,
    title TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := readConfig()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.databaseURL())
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := initialize(db); err != nil {
		return errors.New("database initialization failed")
	}
	logger.Info("database initialized")

	server := &http.Server{
		Addr:              net.JoinHostPort("", cfg.port),
		Handler:           newHandler(db, logger),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      8 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown failed", "error", err)
		}
	}()
	logger.Info("worker listening", "port", cfg.port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type config struct {
	port, host, dbPort, name, user, password string
}

func readConfig() (config, error) {
	c := config{port: os.Getenv("WORKER_PORT"), host: os.Getenv("DB_HOST"), dbPort: os.Getenv("DB_PORT"), name: os.Getenv("DB_NAME"), user: os.Getenv("POSTGRES_USER"), password: os.Getenv("POSTGRES_PASSWORD")}
	if c.port == "" {
		c.port = "8080"
	}
	for _, entry := range []struct{ name, value string }{{"DB_HOST", c.host}, {"DB_PORT", c.dbPort}, {"DB_NAME", c.name}, {"POSTGRES_USER", c.user}, {"POSTGRES_PASSWORD", c.password}} {
		if entry.value == "" {
			return config{}, fmt.Errorf("%s is required", entry.name)
		}
	}
	for _, entry := range []struct{ name, value string }{{"WORKER_PORT", c.port}, {"DB_PORT", c.dbPort}} {
		port, err := strconv.Atoi(entry.value)
		if err != nil || port < 1 || port > 65535 {
			return config{}, fmt.Errorf("%s must be a valid TCP port", entry.name)
		}
	}
	return c, nil
}

func (c config) databaseURL() string {
	u := &url.URL{Scheme: "postgres", User: url.UserPassword(c.user, c.password), Host: net.JoinHostPort(c.host, c.dbPort), Path: "/" + c.name}
	q := u.Query()
	q.Set("sslmode", "disable") // The lab database is reached over its internal service network.
	u.RawQuery = q.Encode()
	return u.String()
}

func initialize(db *sql.DB) error {
	deadline := time.Now().Add(15 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := db.PingContext(ctx)
		if err == nil {
			_, err = db.ExecContext(ctx, schema)
		}
		cancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if s[i] != '-' {
				return false
			}
		} else if !((s[i] >= '0' && s[i] <= '9') || (s[i] >= 'a' && s[i] <= 'f') || (s[i] >= 'A' && s[i] <= 'F')) {
			return false
		}
	}
	return true
}

func normalizeTitle(s string) (string, bool) {
	s = strings.TrimFunc(s, unicode.IsSpace)
	n := utf8.RuneCountInString(s)
	return s, n >= 1 && n <= 120 && !strings.ContainsRune(s, 0)
}

func parseTitle(r *http.Request) (string, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || len(params) > 1 || (len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8")) {
		return "", errors.New("Content-Type must be application/json")
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 4096))
	if err != nil || !utf8.Valid(body) {
		return "", errors.New("request body must be valid UTF-8 JSON of at most 4 KiB")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	opening, err := dec.Token()
	if err != nil || opening != json.Delim('{') {
		return "", errors.New("request must contain one JSON object")
	}
	key, err := dec.Token()
	if err != nil || key != "title" {
		return "", errors.New("request must contain a string title")
	}
	var title string
	if err := dec.Decode(&title); err != nil {
		return "", errors.New("request must contain a string title")
	}
	closing, err := dec.Token()
	if err != nil || closing != json.Delim('}') {
		return "", errors.New("request must contain only a title")
	}
	if _, err := dec.Token(); err != io.EOF {
		return "", errors.New("request must contain one JSON object")
	}
	title, valid := normalizeTitle(title)
	if !valid {
		return "", errors.New("title must contain 1 to 120 Unicode code points and no NUL")
	}
	return title, nil
}

type task struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
}

type handler struct {
	db      *sql.DB
	logger  *slog.Logger
	metrics *metrics
}

func newHandler(db *sql.DB, logger *slog.Logger) http.Handler {
	h := &handler{db: db, logger: logger, metrics: newMetrics()}
	return http.HandlerFunc(h.serve)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	route := "unmatched"
	switch {
	case r.URL.Path == "/tasks":
		route = "/tasks"
	case strings.HasPrefix(r.URL.Path, "/tasks/") && len(r.URL.Path) > len("/tasks/") && !strings.Contains(r.URL.Path[len("/tasks/"):], "/"):
		route = "/tasks/{id}"
	case r.URL.Path == "/healthz":
		route = "/healthz"
	case r.URL.Path == "/readyz":
		route = "/readyz"
	case r.URL.Path == "/metrics":
		route = "/metrics"
	}
	wrapper := &statusWriter{ResponseWriter: w}
	switch route {
	case "/tasks":
		switch r.Method {
		case http.MethodGet:
			h.list(wrapper, r)
		case http.MethodPost:
			h.create(wrapper, r)
		default:
			methodNotAllowed(wrapper, "GET, POST")
		}
	case "/tasks/{id}":
		if r.Method != http.MethodDelete {
			methodNotAllowed(wrapper, "DELETE")
		} else {
			h.delete(wrapper, r)
		}
	case "/healthz":
		if r.Method != http.MethodGet {
			methodNotAllowed(wrapper, "GET")
		} else {
			writeJSON(wrapper, http.StatusOK, map[string]string{"status": "ok"})
		}
	case "/readyz":
		if r.Method != http.MethodGet {
			methodNotAllowed(wrapper, "GET")
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			if err := h.db.PingContext(ctx); err != nil {
				writeError(wrapper, http.StatusServiceUnavailable, "service_unavailable", "database unavailable")
			} else {
				writeJSON(wrapper, http.StatusOK, map[string]string{"status": "ok"})
			}
		}
	case "/metrics":
		if r.Method != http.MethodGet {
			methodNotAllowed(wrapper, "GET")
		} else {
			h.metrics.write(wrapper)
		}
	default:
		writeError(wrapper, http.StatusNotFound, "not_found", "route not found")
	}
	status := wrapper.status
	if status == 0 {
		status = http.StatusOK
	}
	method := r.Method
	if method != http.MethodGet && method != http.MethodPost && method != http.MethodDelete {
		method = "OTHER"
	}
	h.metrics.observe(method, route, status, time.Since(start).Seconds())
	h.logger.Info("request", "method", method, "route", route, "status", status, "duration_ms", time.Since(start).Milliseconds())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}{code, message})
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

func (h *handler) dbError(w http.ResponseWriter, err error) {
	// A failed ping identifies connectivity errors without exposing SQL details to clients.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if errors.Is(err, context.DeadlineExceeded) || h.db.PingContext(ctx) != nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "database unavailable")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "database operation failed")
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	rows, err := h.db.QueryContext(ctx, "SELECT id, title, created_at FROM tasks ORDER BY created_at DESC, id DESC")
	if err != nil {
		h.dbError(w, err)
		return
	}
	defer rows.Close()
	tasks := make([]task, 0)
	for rows.Next() {
		var t task
		var created time.Time
		if err := rows.Scan(&t.ID, &t.Title, &created); err != nil {
			h.dbError(w, err)
			return
		}
		t.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		h.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Tasks []task `json:"tasks"`
	}{tasks})
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	title, err := parseTitle(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	id, err := newUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create task")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	var created time.Time
	err = h.db.QueryRowContext(ctx, "INSERT INTO tasks (id, title) VALUES ($1, $2) RETURNING created_at", id, title).Scan(&created)
	if err != nil {
		h.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, task{ID: id, Title: title, CreatedAt: created.UTC().Format(time.RFC3339Nano)})
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/tasks/")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "validation_error", "id must be a canonical UUID")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	result, err := h.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = $1", strings.ToLower(id))
	if err != nil {
		h.dbError(w, err)
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "database operation failed")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "not_found", "task not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type metricKey struct {
	method, route string
	status        int
}

type metricValue struct {
	count   uint64
	sum     float64
	buckets [10]uint64
}

type metrics struct {
	mu   sync.Mutex
	data map[metricKey]metricValue
}

func newMetrics() *metrics { return &metrics{data: make(map[metricKey]metricValue)} }

func (m *metrics) observe(method, route string, status int, seconds float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := metricKey{method, route, status}
	v := m.data[k]
	v.count++
	v.sum += seconds
	for i, bound := range buckets {
		if seconds <= bound {
			v.buckets[i]++
		}
	}
	m.data[k] = v
}

func (m *metrics) write(w http.ResponseWriter) {
	m.mu.Lock()
	snapshot := make(map[metricKey]metricValue, len(m.data))
	for k, v := range m.data {
		snapshot[k] = v
	}
	m.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintln(w, "# HELP kubelab_http_requests_total Total HTTP requests handled by KubeLab.")
	fmt.Fprintln(w, "# TYPE kubelab_http_requests_total counter")
	for k, v := range snapshot {
		fmt.Fprintf(w, "kubelab_http_requests_total{service=\"worker\",method=%q,route=%q,status=\"%d\"} %d\n", k.method, k.route, k.status, v.count)
	}
	fmt.Fprintln(w, "# HELP kubelab_http_request_duration_seconds HTTP request duration in seconds.")
	fmt.Fprintln(w, "# TYPE kubelab_http_request_duration_seconds histogram")
	for k, v := range snapshot {
		labels := fmt.Sprintf("service=\"worker\",method=%q,route=%q,status=\"%d\"", k.method, k.route, k.status)
		for i, bound := range buckets {
			fmt.Fprintf(w, "kubelab_http_request_duration_seconds_bucket{%s,le=\"%g\"} %d\n", labels, bound, v.buckets[i])
		}
		fmt.Fprintf(w, "kubelab_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, v.count)
		fmt.Fprintf(w, "kubelab_http_request_duration_seconds_sum{%s} %g\n", labels, v.sum)
		fmt.Fprintf(w, "kubelab_http_request_duration_seconds_count{%s} %d\n", labels, v.count)
	}
}
