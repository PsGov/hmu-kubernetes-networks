package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const exampleID = "550e8400-e29b-41d4-a716-446655440000"
const exampleTask = `{"id":"` + exampleID + `","title":"Learn Kubernetes","created_at":"2026-01-01T12:00:00Z"}`

func testApp(workerURL string) *application {
	return newApplication(workerURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func call(app *application, method, path, body, contentType string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

func expectError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, status, response.Body.String())
	}
	var got apiError
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil || got.Code != code || got.Message == "" {
		t.Fatalf("error response = %s, decode error: %v", response.Body.String(), err)
	}
	if response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("wrong content type: %s", response.Header().Get("Content-Type"))
	}
}

func TestTaskProxy(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch {
		case r.Method == "GET" && r.URL.Path == "/tasks":
			_, _ = io.WriteString(w, `{"tasks":[`+exampleTask+`]}`)
		case r.Method == "POST" && r.URL.Path == "/tasks":
			if r.Header.Get("Content-Type") != "application/json; charset=utf-8" {
				t.Errorf("forwarded content type = %q", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"title":"Learn Kubernetes"}` {
				t.Errorf("forwarded body = %s", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, exampleTask)
		case r.Method == "DELETE" && r.URL.Path == "/tasks/"+strings.ToUpper(exampleID):
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer worker.Close()
	app := testApp(worker.URL)
	for _, test := range []struct {
		method, path, body, contentType string
		status                          int
	}{
		{"GET", "/api/tasks", "", "", 200},
		{"POST", "/api/tasks", `{"title":"  Learn Kubernetes  "}`, "application/json; charset=utf-8", 201},
		{"DELETE", "/api/tasks/" + strings.ToUpper(exampleID), "", "", 204},
	} {
		response := call(app, test.method, test.path, test.body, test.contentType)
		if response.Code != test.status {
			t.Fatalf("%s %s: status %d, body %s", test.method, test.path, response.Code, response.Body.String())
		}
		if test.status == 204 && response.Body.Len() != 0 {
			t.Fatal("204 had a body")
		}
	}
}

func TestValidationAndRouting(t *testing.T) {
	app := testApp("http://127.0.0.1:1")
	for _, test := range []struct{ body, contentType string }{
		{`{"title":" "}`, "application/json"},
		{`{"title":"` + strings.Repeat("é", 121) + `"}`, "application/json"},
		{`{"title":"ok","extra":1}`, "application/json"},
		{`{"title":3}`, "application/json"},
		{`{"title":"ok"} {}`, "application/json"},
		{`{"title":"ok"}`, "text/plain"},
		{`{"title":"ok"}`, "application/json; charset=iso-8859-1"},
		{`{"title":"ok"}`, "application/json; foo=bar"},
		{`{"title":"ok"}`, "application/json"},
		{`{"title":"` + strings.Repeat("x", maxTaskBody) + `"}`, "application/json"},
		{`{"title":"\u0000"}`, "application/json"},
	} {
		if test.body == `{"title":"ok"}` && test.contentType == "application/json" {
			continue
		}
		expectError(t, call(app, "POST", "/api/tasks", test.body, test.contentType), 400, "validation_error")
	}
	expectError(t, call(app, "DELETE", "/api/tasks/invalid", "", ""), 400, "validation_error")
	expectError(t, call(app, "GET", "/missing", "", ""), 404, "not_found")
	response := call(app, "PUT", "/api/tasks", "", "")
	expectError(t, response, 405, "method_not_allowed")
	if response.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("Allow = %q", response.Header().Get("Allow"))
	}
}

func TestWorkerErrors(t *testing.T) {
	for _, test := range []struct {
		name              string
		status            int
		contentType, body string
		wantStatus        int
		wantCode          string
	}{
		{"not found", 404, "application/json; charset=utf-8", `{"error":"not_found","message":"task not found"}`, 404, "not_found"},
		{"database failure", 500, "application/json; charset=utf-8", `{"error":"internal_error","message":"query failed"}`, 500, "internal_error"},
		{"database unavailable", 503, "application/json; charset=utf-8", `{"error":"service_unavailable","message":"database unavailable"}`, 503, "service_unavailable"},
		{"wrong status", 202, "application/json; charset=utf-8", `{}`, 502, "bad_gateway"},
		{"wrong content type", 404, "text/plain", "oops", 502, "bad_gateway"},
		{"wrong error code", 404, "application/json; charset=utf-8", `{"error":"internal_error","message":"oops"}`, 502, "bad_gateway"},
		{"extra error field", 404, "application/json; charset=utf-8", `{"error":"not_found","message":"oops","sql":"secret"}`, 502, "bad_gateway"},
		{"invalid json", 404, "application/json; charset=utf-8", `{`, 502, "bad_gateway"},
	} {
		t.Run(test.name, func(t *testing.T) {
			worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer worker.Close()
			response := call(testApp(worker.URL), "DELETE", "/api/tasks/"+exampleID, "", "")
			expectError(t, response, test.wantStatus, test.wantCode)
			if strings.Contains(response.Body.String(), "secret") {
				t.Fatal("upstream payload leaked")
			}
		})
	}
}

func TestInvalidSuccessAndRedirect(t *testing.T) {
	for _, body := range []string{
		`{"tasks":null}`, `{"tasks":[{"id":"bad","title":"x","created_at":"2026-01-01T12:00:00Z"}]}`,
		`{"tasks":[] ,"other":"unexpected"}`, `{"tasks":[` + exampleTask + `]} trailing`,
	} {
		worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = io.WriteString(w, body)
		}))
		response := call(testApp(worker.URL), "GET", "/api/tasks", "", "")
		expectError(t, response, 502, "bad_gateway")
		worker.Close()
	}
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com", http.StatusFound)
	}))
	defer worker.Close()
	expectError(t, call(testApp(worker.URL), "GET", "/api/tasks", "", ""), 502, "bad_gateway")
}

func TestUnavailableAndLocalRoutes(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	app := testApp(worker.URL)
	app.client.Timeout = 20 * time.Millisecond
	expectError(t, call(app, "GET", "/api/tasks", "", ""), 503, "service_unavailable")
	worker.CloseClientConnections()
	worker.Close()
	response := call(app, "GET", "/readyz", "", "")
	if response.Code != 200 || response.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("readiness = %d %s", response.Code, response.Body.String())
	}
	response = call(app, "GET", "/healthz", "", "")
	if response.Code != 200 {
		t.Fatalf("health status = %d", response.Code)
	}
	response = call(app, "GET", "/api/whoami", "", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"pod_name":"local-api"`) {
		t.Fatalf("whoami = %d %s", response.Code, response.Body.String())
	}
	response = call(app, "GET", "/metrics", "", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `kubelab_http_requests_total{service="api",method="GET",route="/api/tasks",status="503"} 1`) {
		t.Fatalf("metrics = %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), worker.URL) {
		t.Fatal("metrics contain upstream URL")
	}
}
