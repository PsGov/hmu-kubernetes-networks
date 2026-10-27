package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxTaskBody    = 4 * 1024
	maxWorkerReply = 8 * 1024 * 1024
	workerTimeout  = 5 * time.Second
)

type application struct {
	workerURL string
	client    *http.Client
	identity  identity
	metrics   *metrics
	logger    *slog.Logger
}

type identity struct {
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	NodeName     string `json:"node_name"`
	PodIP        string `json:"pod_ip"`
}

type apiError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

type task struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	workerURL, err := parseWorkerURL(os.Getenv("WORKER_URL"))
	if err != nil {
		logger.Error("invalid WORKER_URL", "error", err)
		os.Exit(1)
	}
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		logger.Error("invalid API_PORT")
		os.Exit(1)
	}
	a := newApplication(workerURL, logger)
	server := &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           a,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	logger.Info("api listening", "port", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("api server failed", "error", err)
		os.Exit(1)
	}
}

func parseWorkerURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("must be an HTTP base URL without credentials, path or query")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func localValue(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func newApplication(workerURL string, logger *slog.Logger) *application {
	return &application{
		workerURL: workerURL,
		client: &http.Client{Timeout: workerTimeout, Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   2 * time.Second,
			ResponseHeaderTimeout: 4 * time.Second,
			MaxIdleConns:          20,
			IdleConnTimeout:       30 * time.Second,
		}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		identity: identity{
			PodName:      localValue("POD_NAME", "local-api"),
			PodNamespace: localValue("POD_NAMESPACE", "local"),
			NodeName:     localValue("NODE_NAME", "local-node"),
			PodIP:        localValue("POD_IP", "local"),
		},
		metrics: newMetrics(),
		logger:  logger,
	}
}

func (a *application) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	tracked := &statusWriter{ResponseWriter: w}
	route := routeFor(r.URL.Path)
	defer func() {
		status := tracked.status
		if status == 0 {
			status = http.StatusOK
		}
		a.metrics.observe(metricMethod(r.Method), route, status, time.Since(start).Seconds())
		a.logger.Info("http request", "method", metricMethod(r.Method), "route", route, "status", status, "duration_ms", time.Since(start).Milliseconds())
	}()

	var allowed string
	switch route {
	case "/api/tasks":
		allowed = "GET, POST"
	case "/api/tasks/{id}":
		allowed = "DELETE"
	case "unmatched":
		writeError(tracked, http.StatusNotFound, "not_found", "route not found")
		return
	default:
		allowed = "GET"
	}
	if !methodAllowed(r.Method, allowed) {
		tracked.Header().Set("Allow", allowed)
		writeError(tracked, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	switch route {
	case "/api/tasks":
		if r.Method == http.MethodPost {
			body, err := validatedTaskBody(r)
			if err != nil {
				writeError(tracked, http.StatusBadRequest, "validation_error", err.Error())
				return
			}
			a.forward(tracked, r, "/tasks", body)
			return
		}
		a.forward(tracked, r, "/tasks", nil)
	case "/api/tasks/{id}":
		id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
		if !validUUID(id) {
			writeError(tracked, http.StatusBadRequest, "validation_error", "id must be a canonical UUID")
			return
		}
		a.forward(tracked, r, "/tasks/"+id, nil)
	case "/api/whoami":
		tracked.Header().Set("X-KubeLab-Pod", a.identity.PodName)
		writeJSON(tracked, http.StatusOK, a.identity)
	case "/healthz", "/readyz":
		writeJSON(tracked, http.StatusOK, struct {
			Status string `json:"status"`
		}{"ok"})
	case "/metrics":
		a.metrics.serve(tracked)
	}
}

func routeFor(path string) string {
	switch path {
	case "/api/tasks", "/api/whoami", "/healthz", "/readyz", "/metrics":
		return path
	}
	if strings.HasPrefix(path, "/api/tasks/") && len(path) > len("/api/tasks/") && !strings.Contains(path[len("/api/tasks/"):], "/") {
		return "/api/tasks/{id}"
	}
	return "unmatched"
}

func methodAllowed(method, allowed string) bool {
	for _, candidate := range strings.Split(allowed, ", ") {
		if method == candidate {
			return true
		}
	}
	return false
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodDelete:
		return method
	default:
		return "OTHER"
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Code: code, Message: message})
}

func validJSONType(header string) bool {
	mediaType, params, err := mime.ParseMediaType(header)
	if err != nil || mediaType != "application/json" || len(params) > 1 {
		return false
	}
	if len(params) == 0 {
		return true
	}
	return strings.EqualFold(params["charset"], "utf-8")
}

func validatedTaskBody(r *http.Request) ([]byte, error) {
	if !validJSONType(r.Header.Get("Content-Type")) {
		return nil, errors.New("Content-Type must be application/json with optional charset=utf-8")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxTaskBody+1))
	if err != nil {
		return nil, errors.New("could not read request body")
	}
	if len(body) > maxTaskBody {
		return nil, errors.New("request body exceeds 4 KiB")
	}
	if !utf8.Valid(body) {
		return nil, errors.New("request body must be valid UTF-8")
	}
	var input map[string]json.RawMessage
	if err := decodeExact(body, &input); err != nil || len(input) != 1 || input["title"] == nil {
		return nil, errors.New("request must be one JSON object with a string title and no other fields")
	}
	var rawTitle *string
	if err := json.Unmarshal(input["title"], &rawTitle); err != nil || rawTitle == nil {
		return nil, errors.New("request must be one JSON object with a string title and no other fields")
	}
	title := strings.TrimFunc(*rawTitle, unicode.IsSpace)
	if !validTitle(title) {
		return nil, errors.New("title must contain 1 to 120 Unicode code points and no NUL")
	}
	return json.Marshal(struct {
		Title string `json:"title"`
	}{title})
}

func validTitle(title string) bool {
	return utf8.ValidString(title) && !strings.ContainsRune(title, 0) && utf8.RuneCountInString(title) >= 1 && utf8.RuneCountInString(title) <= 120 && title == strings.TrimFunc(title, unicode.IsSpace)
}

func validUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func decodeExact(body []byte, dst any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func exactFields(body []byte, fields ...string) bool {
	var object map[string]json.RawMessage
	if decodeExact(body, &object) != nil || len(object) != len(fields) {
		return false
	}
	for _, field := range fields {
		if object[field] == nil {
			return false
		}
	}
	return true
}

func (a *application) forward(w http.ResponseWriter, original *http.Request, path string, body []byte) {
	ctx, cancel := context.WithTimeout(original.Context(), workerTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	request, err := http.NewRequestWithContext(ctx, original.Method, a.workerURL+path, reader)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create Worker request")
		return
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	response, err := a.client.Do(request)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Worker is unavailable")
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent && original.Method == http.MethodDelete {
		one := make([]byte, 1)
		n, readErr := response.Body.Read(one)
		if response.ContentLength > 0 || n != 0 || (readErr != nil && readErr != io.EOF) {
			writeError(w, http.StatusBadGateway, "bad_gateway", "Worker returned an invalid response")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !validJSONType(response.Header.Get("Content-Type")) {
		writeError(w, http.StatusBadGateway, "bad_gateway", "Worker returned an invalid response")
		return
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxWorkerReply+1))
	if err != nil && (errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded)) {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Worker is unavailable")
		return
	}
	if err != nil || len(data) > maxWorkerReply || !utf8.Valid(data) || !validWorkerResponse(original.Method, response.StatusCode, data) {
		writeError(w, http.StatusBadGateway, "bad_gateway", "Worker returned an invalid response")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(data)
}

func validWorkerResponse(method string, status int, body []byte) bool {
	switch status {
	case http.StatusOK:
		if method != http.MethodGet {
			return false
		}
		if !exactFields(body, "tasks") {
			return false
		}
		var list struct {
			Tasks *[]json.RawMessage `json:"tasks"`
		}
		if decodeExact(body, &list) != nil || list.Tasks == nil {
			return false
		}
		for _, raw := range *list.Tasks {
			var item task
			if !exactFields(raw, "id", "title", "created_at") || decodeExact(raw, &item) != nil || !validTask(item) {
				return false
			}
		}
		return true
	case http.StatusCreated:
		if method != http.MethodPost {
			return false
		}
		var item task
		return exactFields(body, "id", "title", "created_at") && decodeExact(body, &item) == nil && validTask(item)
	case http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError, http.StatusServiceUnavailable:
		if status == http.StatusNotFound && method != http.MethodDelete {
			return false
		}
		var failure apiError
		if !exactFields(body, "error", "message") || decodeExact(body, &failure) != nil || failure.Message == "" {
			return false
		}
		codes := map[int]string{400: "validation_error", 404: "not_found", 500: "internal_error", 503: "service_unavailable"}
		return failure.Code == codes[status]
	default:
		return false
	}
}

func validTask(item task) bool {
	if !validUUID(item.ID) || item.ID != strings.ToLower(item.ID) || item.ID[14] != '4' || !strings.Contains("89ab", string(item.ID[19])) || !validTitle(item.Title) {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, item.CreatedAt)
	return err == nil && parsed.Location() == time.UTC && strings.HasSuffix(item.CreatedAt, "Z")
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
