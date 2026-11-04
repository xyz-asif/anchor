package interests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------- unit tests ----------

func TestValidateInterests(t *testing.T) {
	got, err := ValidateInterests(&[]string{"Travel", "#travel", " Web Design "})
	require.NoError(t, err)
	require.Equal(t, []string{"travel", "web-design"}, got, "normalised like anchor tags, duplicates once")

	got, err = ValidateInterests(&[]string{})
	require.NoError(t, err)
	require.Empty(t, got, "an empty list clears interests")

	_, err = ValidateInterests(nil)
	require.ErrorContains(t, err, `Please send your interests as {"tags"`)
	_, err = ValidateInterests(&[]string{"travel", "x"})
	require.EqualError(t, err, `"x" isn't a valid interest. Interests are 2 to 30 letters or numbers`, "invalid tags are reported, not silently dropped")

	var eleven []string
	for i := 0; i < 11; i++ {
		eleven = append(eleven, fmt.Sprintf("topic%d", i))
	}
	_, err = ValidateInterests(&eleven)
	require.EqualError(t, err, "You can pick at most 10 interests (you picked 11)")
}

// ---------- integration tests (need AUTH_TEST_MONGO_URI) ----------

type env struct {
	t      *testing.T
	db     *mongo.Database
	router *gin.Engine
}

func newEnv(t *testing.T) *env {
	uri := os.Getenv("AUTH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("AUTH_TEST_MONGO_URI not set; skipping integration test")
	}
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	require.NoError(t, err)
	db := client.Database(fmt.Sprintf("interests_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		time.Sleep(50 * time.Millisecond)
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	cfg := &config.Config{AppEnv: "test", DevMode: true, JWTSecret: "test-secret-test-secret-test-secret-123",
		JWTExpireHours: 1, RefreshTokenExpireHours: 24}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	auth.RegisterRoutes(api, db, cfg, nil, nil)
	anchors.RegisterRoutes(api, db, cfg, nil, notifications.GetService(db))
	RegisterRoutes(api, db, cfg)
	return &env{t: t, db: db, router: r}
}

func (e *env) do(method, path, token string, body interface{}) (int, map[string]interface{}) {
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

func (e *env) login(email string) (string, primitive.ObjectID) {
	code, out := e.do("POST", "/api/v1/auth/dev-login", "", map[string]string{"email": email})
	require.Equal(e.t, 200, code, out)
	data := out["data"].(map[string]interface{})
	id, _ := primitive.ObjectIDFromHex(data["user"].(map[string]interface{})["id"].(string))
	return data["token"].(string), id
}

func (e *env) create(token, visibility string, tagList ...string) string {
	code, out := e.do("POST", "/api/v1/anchors", token, map[string]interface{}{"title": "Anchor " + strings.Join(tagList, " "), "visibility": visibility, "tags": tagList})
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func (e *env) suggestions(token, query string) map[string]interface{} {
	code, out := e.do("GET", "/api/v1/interests/suggested"+query, token, nil)
	require.Equal(e.t, 200, code, out)
	return out["data"].(map[string]interface{})
}

func names(data map[string]interface{}) []string {
	var out []string
	for _, c := range data["categories"].([]interface{}) {
		out = append(out, c.(map[string]interface{})["name"].(string))
	}
	return out
}

func TestSaveAndClearInterests(t *testing.T) {
	e := newEnv(t)
	tok, _ := e.login("s@example.com")

	code, out := e.do("POST", "/api/v1/users/me/interests", tok, map[string]interface{}{"tags": []string{"travel", "x"}})
	require.Equal(t, 400, code)
	require.Equal(t, `"x" isn't a valid interest. Interests are 2 to 30 letters or numbers`, out["message"])

	code, out = e.do("POST", "/api/v1/users/me/interests", tok, map[string]interface{}{"tags": []string{"#Travel", "Books"}})
	require.Equal(t, 200, code, out)
	require.Equal(t, []interface{}{"travel", "books"}, out["data"].(map[string]interface{})["interests"])

	code, out = e.do("POST", "/api/v1/users/me/interests", tok, map[string]interface{}{"tags": []string{}})
	require.Equal(t, 200, code, out)
	require.Equal(t, "Interests cleared", out["message"])
	require.Empty(t, e.suggestions(tok, "")["selected"])
}

func TestPrivateLikedAnchorsDontLeakIntoSuggestions(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("o@example.com")
	tok, me := e.login("me@example.com")
	a := e.create(ownerTok, "public", "secret-project")
	e.create(ownerTok, "public", "gardening")
	aid, _ := primitive.ObjectIDFromHex(a)
	_, err := e.db.Collection("likes").InsertOne(context.Background(), bson.M{"anchorId": aid, "userId": me, "createdAt": time.Now()})
	require.NoError(t, err)
	require.Contains(t, names(e.suggestions(tok, "")), "secret-project")

	// The owner makes it private: its tags must stop showing up in my suggestions
	code, _ := e.do("PATCH", "/api/v1/anchors/"+a, ownerTok, map[string]string{"visibility": "private"})
	require.Equal(t, 200, code)
	require.NotContains(t, names(e.suggestions(tok, "")), "secret-project")
}

func TestSuggestionsAreFullAndStable(t *testing.T) {
	e := newEnv(t)
	tok, me := e.login("n@example.com")
	e.create(tok, "public", "pottery")

	// A legacy anchor with an unnormalised tag counts together with "pottery"
	_, err := e.db.Collection("anchors").InsertOne(context.Background(), bson.M{"userId": me, "title": "Old", "visibility": "public", "tags": []string{"Pottery"}})
	require.NoError(t, err)

	data := e.suggestions(tok, "")
	require.Equal(t, "personalized", data["basedOn"])
	got := names(data)
	require.Len(t, got, 10, "one personal tag is topped up with popular and starter tags")
	require.Equal(t, "pottery", got[0])
	require.Equal(t, 1, strings.Count(strings.Join(got, ","), "pottery"), "case variants are one tag")
	require.Equal(t, got, names(e.suggestions(tok, "")), "same order on every request")

	require.Len(t, names(e.suggestions("", "?limit=50")), 19, "limit is capped at 20; 1 real tag + 18 starter tags exist")
	code, out := e.do("GET", "/api/v1/interests/suggested?limit=abc", "", nil)
	require.Equal(t, 400, code)
	require.Equal(t, `Limit must be a whole number between 1 and 20 (got "abc")`, out["message"])
}
