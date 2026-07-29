package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/api"
	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/config"
	"github.com/jacobrluttrull/gator/internal/database"
	"github.com/jacobrluttrull/gator/internal/testsupport"
)

func apiLogin(t *testing.T, h http.Handler, name, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"name": name, "password": password})
	if err != nil {
		t.Fatalf("marshaling login body: %v", err)
	}
	req := httptest.NewRequest("POST", "/v1/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestSetPasswordUpgradesCLIOnlyUser walks issue #6's acceptance path:
// a CLI-only user is refused API login, runs setpassword from the
// trusted CLI, then logs in over the API and finds their pre-existing
// follows waiting.
func TestSetPasswordUpgradesCLIOnlyUser(t *testing.T) {
	queries := database.New(testsupport.OpenTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()

	// A CLI-registered user: no password hash, with an existing follow.
	user, err := queries.CreateUser(ctx, database.CreateUserParams{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Name: "cliuser",
	})
	if err != nil {
		t.Fatalf("seeding CLI-only user: %v", err)
	}
	feed, err := queries.CreateFeed(ctx, database.CreateFeedParams{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
		Name: "Boot.dev Blog", Url: "https://blog.boot.dev/index.xml", UserID: user.ID,
	})
	if err != nil {
		t.Fatalf("seeding feed: %v", err)
	}
	if _, err := queries.CreateFeedFollow(ctx, database.CreateFeedFollowParams{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, UserID: user.ID, FeedID: feed.ID,
	}); err != nil {
		t.Fatalf("seeding follow: %v", err)
	}

	h := api.New(queries)
	if rr := apiLogin(t, h, "cliuser", "first-pw"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("CLI-only user login status = %d, want %d; body: %s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}

	// setpassword runs as the current CLI user via the LoggedIn wrapper,
	// exactly as registered in cmd/gator.
	s := &cli.State{Config: &config.Config{CurrentUserName: "cliuser"}, DB: queries}
	setpassword := cli.LoggedIn(SetPassword)
	if err := setpassword(s, cli.Command{Name: "setpassword", Args: []string{"first-pw"}}); err != nil {
		t.Fatalf("setpassword: %v", err)
	}

	rr := apiLogin(t, h, "cliuser", "first-pw")
	if rr.Code != http.StatusOK {
		t.Fatalf("login after setpassword status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var got struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got.APIKey == "" {
		t.Fatalf("login response missing api_key: %v; body: %s", err, rr.Body.String())
	}

	// The pre-existing follow is visible over the authenticated API.
	req := httptest.NewRequest("GET", "/v1/follows", nil)
	req.Header.Set("Authorization", "ApiKey "+got.APIKey)
	fr := httptest.NewRecorder()
	h.ServeHTTP(fr, req)
	if fr.Code != http.StatusOK {
		t.Fatalf("GET /v1/follows status = %d, want %d; body: %s", fr.Code, http.StatusOK, fr.Body.String())
	}
	var follows []struct {
		FeedName string `json:"feed_name"`
		UserName string `json:"user_name"`
	}
	if err := json.Unmarshal(fr.Body.Bytes(), &follows); err != nil {
		t.Fatalf("follows response is not valid JSON: %v; body: %s", err, fr.Body.String())
	}
	if len(follows) != 1 || follows[0].FeedName != "Boot.dev Blog" || follows[0].UserName != "cliuser" {
		t.Fatalf("follows = %+v, want the pre-existing Boot.dev Blog follow for cliuser", follows)
	}

	// Running setpassword again replaces the password with no
	// old-password check — the CLI is trusted (ADR-0001).
	if err := setpassword(s, cli.Command{Name: "setpassword", Args: []string{"second-pw"}}); err != nil {
		t.Fatalf("second setpassword: %v", err)
	}
	if rr := apiLogin(t, h, "cliuser", "first-pw"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("old password after replacement: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if rr := apiLogin(t, h, "cliuser", "second-pw"); rr.Code != http.StatusOK {
		t.Fatalf("new password after replacement: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestSetPasswordRequiresAnArgument(t *testing.T) {
	err := SetPassword(&cli.State{}, cli.Command{Name: "setpassword"}, database.User{})
	if err == nil {
		t.Fatal("SetPassword with no args returned nil; want an error")
	}
}

// An empty password would bcrypt to a valid hash that an empty-password
// API login could then match, breaking login's "empty credentials never
// log in" invariant.
func TestSetPasswordRejectsEmptyPassword(t *testing.T) {
	err := SetPassword(&cli.State{}, cli.Command{Name: "setpassword", Args: []string{""}}, database.User{})
	if err == nil {
		t.Fatal("SetPassword with an empty password returned nil; want an error")
	}
}
