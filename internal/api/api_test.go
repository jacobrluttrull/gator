package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"github.com/jacobrluttrull/gator/internal/api"
	"github.com/jacobrluttrull/gator/internal/database"
	"github.com/jacobrluttrull/gator/internal/testsupport"
)

// testHandler opens the shared test database and returns the /v1 API
// handler plus the underlying DB for test seeding.
func testHandler(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db := testsupport.OpenTestDB(t)
	return api.New(database.New(db)), db
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestRegisterCreatesUser(t *testing.T) {
	h, _ := testHandler(t)

	rr := doJSON(t, h, "POST", "/v1/register", `{"name": "alice", "password": "s3cret-pw"}`)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v; body: %s", err, rr.Body.String())
	}
	if got["name"] != "alice" {
		t.Errorf("name = %v, want %q", got["name"], "alice")
	}
	if id, _ := got["id"].(string); id == "" {
		t.Errorf("id missing or empty in response: %s", rr.Body.String())
	}
	lower := strings.ToLower(rr.Body.String())
	if strings.Contains(lower, "password") || strings.Contains(lower, "s3cret-pw") {
		t.Errorf("response leaks password material: %s", rr.Body.String())
	}
}

func TestRegisterDuplicateNameIsClean4xx(t *testing.T) {
	h, _ := testHandler(t)

	first := doJSON(t, h, "POST", "/v1/register", `{"name": "alice", "password": "s3cret-pw"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first register status = %d, want %d", first.Code, http.StatusCreated)
	}

	second := doJSON(t, h, "POST", "/v1/register", `{"name": "alice", "password": "other-pw"}`)
	if second.Code != http.StatusConflict {
		t.Fatalf("duplicate register status = %d, want %d; body: %s", second.Code, http.StatusConflict, second.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(second.Body.Bytes(), &got); err != nil {
		t.Fatalf("error response is not valid JSON: %v; body: %s", err, second.Body.String())
	}
	if got["error"] == "" {
		t.Errorf(`error response missing "error" key: %s`, second.Body.String())
	}
}

func TestRegisterMissingFieldsIs400(t *testing.T) {
	h, _ := testHandler(t)

	for _, body := range []string{`{"name": "bob"}`, `{"password": "pw"}`, `not json`} {
		rr := doJSON(t, h, "POST", "/v1/register", body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("register with body %q: status = %d, want %d", body, rr.Code, http.StatusBadRequest)
		}
		var got map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
			t.Errorf("register with body %q: want error-shape JSON, got: %s", body, rr.Body.String())
		}
	}
}
