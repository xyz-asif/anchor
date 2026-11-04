package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xyz-asif/gotodo/internal/config"
	idToken "github.com/xyz-asif/gotodo/internal/pkg/jwt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------- unit tests (no database) ----------

func TestUsernameBase(t *testing.T) {
	cases := map[string]string{
		"John Doe":                    "john_doe",
		"":                            "user_",
		"शर्मा":                       "user_",
		"ab":                          "user_ab",
		"admin":                       "admin_1",
		"averyveryverylongname12345x": "averyveryverylongnam",
	}
	for in, want := range cases {
		got := GenerateUniqueUsername(in)
		require.Equal(t, want, got, "input %q", in)
		require.NoError(t, ValidateUsername(got), "generated %q must be valid", got)
	}
}

func TestWithUsernameSuffixStaysWithinLimit(t *testing.T) {
	got := withUsernameSuffix("averyveryverylongnam", "123456")
	require.Len(t, got, usernameMaxLen)
	require.True(t, strings.HasSuffix(got, "123456"))
	require.NoError(t, ValidateUsername(got))
}

func TestCleanDisplayName(t *testing.T) {
	require.Equal(t, "Jane", cleanDisplayName("  Jane \n", "fallback"))
	require.Equal(t, "fallback", cleanDisplayName("​​​", "fallback"))
	require.Equal(t, "fallback", cleanDisplayName("x", "fallback"))
	require.Equal(t, 50, len([]rune(cleanDisplayName(strings.Repeat("é", 80), "f"))))
	require.Equal(t, "👨‍👩‍👧 family", cleanDisplayName("👨‍👩‍👧 family", "f"))
}

func TestBearerToken(t *testing.T) {
	for _, h := range []string{"Bearer abc", "bearer abc", "BEARER   abc", " Bearer abc "} {
		tok, ok := bearerToken(h)
		require.True(t, ok, h)
		require.Equal(t, "abc", tok)
	}
	for _, h := range []string{"Bearer", "Basic abc", "Bearer a b", "abc"} {
		_, ok := bearerToken(h)
		require.False(t, ok, h)
	}
}

// ---------- integration tests (need AUTH_TEST_MONGO_URI, e.g. a throwaway local mongod) ----------

const testSecret = "test-secret-test-secret-test-secret-123"

type testEnv struct {
	t      *testing.T
	db     *mongo.Database
	repo   *Repository
	router *gin.Engine
	cfg    *config.Config
}

func newTestEnv(t *testing.T) *testEnv {
	uri := os.Getenv("AUTH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("AUTH_TEST_MONGO_URI not set; skipping integration test")
	}
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	require.NoError(t, err)
	db := client.Database(fmt.Sprintf("auth_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	cfg := &config.Config{
		AppEnv: "test", DevMode: true, JWTSecret: testSecret,
		JWTExpireHours: 1, RefreshTokenExpireHours: 24,
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r.Group("/api/v1"), db, cfg, nil, nil)
	return &testEnv{t: t, db: db, repo: NewRepository(db), router: r, cfg: cfg}
}

func (e *testEnv) do(method, path, token string, body interface{}) (int, map[string]interface{}) {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (e *testEnv) devLogin(email string) (access, refresh string, user map[string]interface{}) {
	code, out := e.do("POST", "/api/v1/auth/dev-login", "", map[string]string{"email": email, "displayName": "x"})
	require.Equal(e.t, 200, code, out)
	data := out["data"].(map[string]interface{})
	return data["token"].(string), data["refreshToken"].(string), data["user"].(map[string]interface{})
}

func errorCode(out map[string]interface{}) string {
	if c, ok := out["code"].(string); ok {
		return c
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func TestDevLoginSetsJoinedAtAndCleansName(t *testing.T) {
	e := newTestEnv(t)
	_, _, user := e.devLogin("New@Example.com")
	require.Equal(t, "new@example.com", user["email"])
	require.Equal(t, "Dev User", user["displayName"]) // "x" is too short -> fallback
	joined, err := time.Parse(time.RFC3339, user["joinedAt"].(string))
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), joined, time.Minute)
}

func TestLogoutEndsAccessTokenOfThatSessionOnly(t *testing.T) {
	e := newTestEnv(t)
	accessA, refreshA, _ := e.devLogin("a@example.com")
	accessB, _, _ := e.devLogin("a@example.com") // second device

	// A refreshed access token belongs to the same session
	code, out := e.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refreshToken": refreshA})
	require.Equal(t, 200, code, out)
	refreshedA := out["data"].(map[string]interface{})["accessToken"].(string)

	code, _ = e.do("GET", "/api/v1/auth/me", accessA, nil)
	require.Equal(t, 200, code)

	code, _ = e.do("POST", "/api/v1/auth/logout", "", map[string]string{"refreshToken": refreshA})
	require.Equal(t, 200, code)

	for _, tok := range []string{accessA, refreshedA} {
		code, out = e.do("GET", "/api/v1/auth/me", tok, nil)
		require.Equal(t, 401, code, "access token must stop working after logout")
		require.Equal(t, "TOKEN_REVOKED", errorCode(out))
	}
	code, _ = e.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refreshToken": refreshA})
	require.Equal(t, 401, code)

	// The other device is unaffected
	code, _ = e.do("GET", "/api/v1/auth/me", accessB, nil)
	require.Equal(t, 200, code)
}

func TestRevokeAllEndsEverySession(t *testing.T) {
	e := newTestEnv(t)
	accessA, _, _ := e.devLogin("b@example.com")
	accessB, refreshB, _ := e.devLogin("b@example.com")

	code, _ := e.do("POST", "/api/v1/auth/revoke-all", accessA, nil)
	require.Equal(t, 200, code)
	for _, tok := range []string{accessA, accessB} {
		code, _ = e.do("GET", "/api/v1/auth/me", tok, nil)
		require.Equal(t, 401, code)
	}
	code, _ = e.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refreshToken": refreshB})
	require.Equal(t, 401, code)
}

func TestLegacyAccessTokenWithoutSessionStillWorks(t *testing.T) {
	e := newTestEnv(t)
	_, _, user := e.devLogin("legacy@example.com")
	legacy, err := idToken.GenerateToken(user["id"].(string), "legacy@example.com", idToken.DefaultConfig(testSecret))
	require.NoError(t, err)
	code, _ := e.do("GET", "/api/v1/auth/me", legacy, nil)
	require.Equal(t, 200, code)
}

func TestUsernameChangeIsAtomicallyOnce(t *testing.T) {
	e := newTestEnv(t)
	access, _, _ := e.devLogin("c@example.com")

	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, _ := e.do("PATCH", "/api/v1/auth/username", access, map[string]string{"username": fmt.Sprintf("racer_%d", i)})
			if code == 200 {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, 1, successes, "only one concurrent username change may succeed")
}

func TestUsernameSameAsCurrentIsNoOp(t *testing.T) {
	e := newTestEnv(t)
	access, _, user := e.devLogin("d@example.com")
	code, out := e.do("PATCH", "/api/v1/auth/username", access, map[string]string{"username": user["username"].(string)})
	require.Equal(t, 200, code, out)
	require.Equal(t, false, out["data"].(map[string]interface{})["usernameChanged"])
}

func TestUpdateProfileStoresTrimmedValues(t *testing.T) {
	e := newTestEnv(t)
	access, _, _ := e.devLogin("e@example.com")
	code, out := e.do("PATCH", "/api/v1/users/me", access, map[string]string{"displayName": "  Jane  ", "bio": "  hi\nthere ‮ "})
	require.Equal(t, 200, code, out)
	data := out["data"].(map[string]interface{})
	require.Equal(t, "Jane", data["displayName"])
	require.Equal(t, "hi\nthere", data["bio"])

	code, _ = e.do("PATCH", "/api/v1/users/me", access, map[string]string{"displayName": "​​​"})
	require.Equal(t, 400, code, "an invisible display name must be rejected")
}

func TestProfileHiddenFromUsersTheOwnerBlocked(t *testing.T) {
	e := newTestEnv(t)
	accessA, _, userA := e.devLogin("f@example.com")
	_, _, userB := e.devLogin("g@example.com")
	aID, _ := primitive.ObjectIDFromHex(userA["id"].(string))
	bID, _ := primitive.ObjectIDFromHex(userB["id"].(string))
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": bID}, bson.M{"$set": bson.M{"blockedUsers": []primitive.ObjectID{aID}}})
	require.NoError(t, err)

	code, _ := e.do("GET", "/api/v1/users/"+bID.Hex(), accessA, nil)
	require.Equal(t, 404, code, "blocked viewer must not see the profile")
	code, _ = e.do("GET", "/api/v1/users/"+bID.Hex(), "", nil)
	require.Equal(t, 200, code, "anonymous viewers still can")
}

func TestGoogleAccountLinking(t *testing.T) {
	e := newTestEnv(t)
	h := &Handler{repo: e.repo, config: e.cfg}
	ctx := context.Background()

	// An account linked to another Google account is not taken over by email
	require.NoError(t, e.repo.CreateUser(ctx, &User{GoogleID: "google-1", Email: "h@example.com", Username: "h_user"}))
	_, _, aerr := h.findOrCreateGoogleUser(ctx, &GoogleUser{UID: "google-2", Email: "h@example.com", Name: "H"})
	require.NotNil(t, aerr)
	require.Equal(t, http.StatusConflict, aerr.status)

	// A dev-login placeholder is linked
	require.NoError(t, e.repo.CreateUser(ctx, &User{GoogleID: "dev_x", Email: "i@example.com", Username: "i_user"}))
	u, isNew, aerr := h.findOrCreateGoogleUser(ctx, &GoogleUser{UID: "google-3", Email: "i@example.com", Name: "I"})
	require.Nil(t, aerr)
	require.False(t, isNew)
	require.Equal(t, "google-3", u.GoogleID)

	// Reserved names are never generated
	u, isNew, aerr = h.findOrCreateGoogleUser(ctx, &GoogleUser{UID: "google-4", Email: "admin@example.com", Name: "Admin"})
	require.Nil(t, aerr)
	require.True(t, isNew)
	require.NotEqual(t, "admin", u.Username)
	require.NoError(t, ValidateUsername(u.Username))
}

func TestConcurrentFirstGoogleLoginCreatesOneAccount(t *testing.T) {
	e := newTestEnv(t)
	h := &Handler{repo: e.repo, config: e.cfg}
	ctx := context.Background()

	var wg sync.WaitGroup
	ids := make([]string, 8)
	errs := make([]*authError, 8)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u, _, aerr := h.findOrCreateGoogleUser(ctx, &GoogleUser{UID: "google-race", Email: "race@example.com", Name: "Race"})
			errs[i] = aerr
			if u != nil {
				ids[i] = u.ID.Hex()
			}
		}(i)
	}
	wg.Wait()
	for i := range ids {
		require.Nil(t, errs[i], "login %d failed", i)
		require.Equal(t, ids[0], ids[i])
	}
	n, err := e.db.Collection("users").CountDocuments(ctx, bson.M{"googleId": "google-race"})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}
