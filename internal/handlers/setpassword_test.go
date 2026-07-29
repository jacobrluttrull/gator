package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/api"
	"github.com/jacobrluttrull/gator/internal/auth"
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
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/v1/login", strings.NewReader(string(body)))
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
	db := testsupport.OpenTestDB(t)
	queries := database.New(db)
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

	h := api.New(queries, db)
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
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/v1/follows", nil)
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

// TestSetPasswordRejectsOverlongPassword covers the CLI half of bcrypt's
// 72-byte limit: the command must refuse it and leave the stored hash
// alone rather than reporting an internal hashing failure.
func TestSetPasswordRejectsOverlongPassword(t *testing.T) {
	queries := database.New(testsupport.OpenTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()

	user, err := queries.CreateUser(ctx, database.CreateUserParams{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Name: "cliuser",
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	s := &cli.State{Config: &config.Config{CurrentUserName: "cliuser"}, DB: queries}

	if err := SetPassword(s, cli.Command{Name: "setpassword", Args: []string{"workingpw"}}, user); err != nil {
		t.Fatalf("setpassword with a normal password: %v", err)
	}

	err = SetPassword(s, cli.Command{Name: "setpassword", Args: []string{strings.Repeat("a", 73)}}, user)
	if !errors.Is(err, auth.ErrPasswordTooLong) {
		t.Fatalf("setpassword with a 73-byte password = %v, want ErrPasswordTooLong", err)
	}

	// The rejected attempt must not have disturbed the working password.
	after, err := queries.GetUser(ctx, "cliuser")
	if err != nil {
		t.Fatalf("re-reading user: %v", err)
	}
	if !after.PasswordHash.Valid {
		t.Fatal("password hash was cleared by the rejected setpassword")
	}
	if err := auth.CheckPasswordHash("workingpw", after.PasswordHash.String); err != nil {
		t.Errorf("original password no longer verifies after a rejected setpassword: %v", err)
	}
}

// TestSetPasswordRevokesExistingKeys is the point of the revocation work:
// a user who thinks a key has leaked sets a new password, and the leaked
// key must stop working immediately — not merely stop being reissued.
func TestSetPasswordRevokesExistingKeys(t *testing.T) {
	db := testsupport.OpenTestDB(t)
	queries := database.New(db)
	ctx := context.Background()
	h := api.New(queries, db)

	// A user with a password and two live keys, as if logged in twice.
	if rr := apiRegister(t, h, "leaky", "first-pw"); rr.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	firstKey := apiKeyFromLogin(t, h, "leaky", "first-pw")
	secondKey := apiKeyFromLogin(t, h, "leaky", "first-pw")
	for name, key := range map[string]string{"first": firstKey, "second": secondKey} {
		if !keyAuthenticates(t, h, key) {
			t.Fatalf("%s key doesn't authenticate before setpassword; nothing to revoke", name)
		}
	}

	user, err := queries.GetUser(ctx, "leaky")
	if err != nil {
		t.Fatalf("reading user: %v", err)
	}
	s := &cli.State{Config: &config.Config{CurrentUserName: "leaky"}, DB: queries}
	if err := SetPassword(s, cli.Command{Name: "setpassword", Args: []string{"second-pw"}}, user); err != nil {
		t.Fatalf("setpassword: %v", err)
	}

	// Both pre-existing keys are dead.
	if keyAuthenticates(t, h, firstKey) {
		t.Error("a key issued before setpassword still authenticates after it")
	}
	if keyAuthenticates(t, h, secondKey) {
		t.Error("a second pre-existing key survived setpassword")
	}
	var remaining int
	if err := db.QueryRowContext(context.Background(), `
		select count(*) from api_keys k join users u on u.id = k.user_id
		where u.name = 'leaky'`).Scan(&remaining); err != nil {
		t.Fatalf("counting keys: %v", err)
	}
	if remaining != 0 {
		t.Errorf("api_keys rows for the user after setpassword = %d, want 0", remaining)
	}

	// The password write itself landed, and a key issued after it works.
	if rr := apiLogin(t, h, "leaky", "first-pw"); rr.Code != http.StatusUnauthorized {
		t.Errorf("login with the old password = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	fresh := apiKeyFromLogin(t, h, "leaky", "second-pw")
	if !keyAuthenticates(t, h, fresh) {
		t.Error("a key issued after setpassword doesn't authenticate")
	}
}

// TestSetPasswordRevocationIsPerUser keeps one user's password change from
// logging everybody else out.
func TestSetPasswordRevocationIsPerUser(t *testing.T) {
	db := testsupport.OpenTestDB(t)
	queries := database.New(db)
	ctx := context.Background()
	h := api.New(queries, db)

	for _, name := range []string{"alice", "bob"} {
		if rr := apiRegister(t, h, name, "pw-"+name); rr.Code != http.StatusCreated {
			t.Fatalf("register %s status = %d; body: %s", name, rr.Code, rr.Body.String())
		}
	}
	aliceKey := apiKeyFromLogin(t, h, "alice", "pw-alice")
	bobKey := apiKeyFromLogin(t, h, "bob", "pw-bob")

	alice, err := queries.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("reading alice: %v", err)
	}
	s := &cli.State{Config: &config.Config{CurrentUserName: "alice"}, DB: queries}
	if err := SetPassword(s, cli.Command{Name: "setpassword", Args: []string{"new-pw"}}, alice); err != nil {
		t.Fatalf("setpassword: %v", err)
	}

	if keyAuthenticates(t, h, aliceKey) {
		t.Error("alice's key survived her own setpassword")
	}
	if !keyAuthenticates(t, h, bobKey) {
		t.Error("bob's key was revoked by alice's setpassword")
	}
}

// apiRegister registers a user over the API.
func apiRegister(t *testing.T, h http.Handler, name, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"name": name, "password": password})
	if err != nil {
		t.Fatalf("marshaling register body: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/v1/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// apiKeyFromLogin logs in over the API and returns the issued key.
func apiKeyFromLogin(t *testing.T, h http.Handler, name, password string) string {
	t.Helper()
	rr := apiLogin(t, h, name, password)
	if rr.Code != http.StatusOK {
		t.Fatalf("login %s status = %d, want %d; body: %s", name, rr.Code, http.StatusOK, rr.Body.String())
	}
	var got struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got.APIKey == "" {
		t.Fatalf("login %s returned no api_key: %s", name, rr.Body.String())
	}
	return got.APIKey
}

// keyAuthenticates reports whether a key still works on an authed route.
func keyAuthenticates(t *testing.T, h http.Handler, key string) bool {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/v1/follows", nil)
	req.Header.Set("Authorization", "ApiKey "+key)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	switch rr.Code {
	case http.StatusOK:
		return true
	case http.StatusUnauthorized:
		return false
	default:
		t.Fatalf("GET /v1/follows = %d, want 200 or 401; body: %s", rr.Code, rr.Body.String())
		return false
	}
}
