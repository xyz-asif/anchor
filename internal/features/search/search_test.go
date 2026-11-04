package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
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

func TestParseSearchQueries(t *testing.T) {
	_, err := ParseUserSearchQuery("猫", "", "")
	require.EqualError(t, err, "Please type at least 2 characters to search", "length counts characters, not bytes")
	q, err := ParseUserSearchQuery(strings.Repeat("日", 100), "", "")
	require.NoError(t, err, "100 CJK characters (300 bytes) are allowed")
	require.Equal(t, 20, q.Limit)

	_, err = ParseUserSearchQuery("  ​ ", "", "")
	require.EqualError(t, err, "Please type something to search for")

	a, err := ParseAnchorSearchQuery("  web ​  design ", "#Travel", "RECENT", "2", "500")
	require.NoError(t, err)
	require.Equal(t, "web design", a.Q)
	require.Equal(t, "travel", a.Tag)
	require.Equal(t, SortRecent, a.Sort)
	require.Equal(t, 50, a.Limit, "a large limit is capped, not rejected")

	_, err = ParseAnchorSearchQuery("books", "", "best", "", "")
	require.EqualError(t, err, `"best" isn't a valid sort. Use relevant, recent or popular`)
	_, err = ParseAnchorSearchQuery("books", "", "", "abc", "")
	require.EqualError(t, err, `Page must be a whole number of 1 or more (got "abc")`)
	_, err = ParseUnifiedQuery("books", "posts", "")
	require.EqualError(t, err, `"posts" isn't a search type. Use all, anchors or users`)

	tq, err := ParseTagSearchQuery("#Web Des", "")
	require.NoError(t, err)
	require.Equal(t, "web-des", tq.Q)
	_, err = ParseTagSearchQuery("##", "")
	require.EqualError(t, err, `"##" has no letters or numbers to search tags for`)
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
	db := client.Database(fmt.Sprintf("search_test_%d", time.Now().UnixNano()))
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
	time.Sleep(200 * time.Millisecond) // let the text indexes finish building
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

// login signs in and sets the username, returning token and ID
func (e *env) login(email, username string) (string, primitive.ObjectID) {
	code, out := e.do("POST", "/api/v1/auth/dev-login", "", map[string]string{"email": email})
	require.Equal(e.t, 200, code, out)
	data := out["data"].(map[string]interface{})
	id, _ := primitive.ObjectIDFromHex(data["user"].(map[string]interface{})["id"].(string))
	// Through the repository, like the app, so the search terms follow the new name
	require.NoError(e.t, auth.NewRepository(e.db).UpdateUser(context.Background(), id.Hex(), map[string]interface{}{"username": username}))
	return data["token"].(string), id
}

func (e *env) create(token, title string, tagList ...string) {
	code, out := e.do("POST", "/api/v1/anchors", token, map[string]interface{}{"title": title, "visibility": "public", "tags": tagList})
	require.Equal(e.t, 201, code, out)
}

func (e *env) get(path, token string) map[string]interface{} {
	code, out := e.do("GET", path, token, nil)
	require.Equal(e.t, 200, code, out)
	return out["data"].(map[string]interface{})
}

func usernames(list interface{}) []string {
	var out []string
	for _, u := range list.([]interface{}) {
		out = append(out, u.(map[string]interface{})["username"].(string))
	}
	return out
}

func TestUsernamePrefixSearch(t *testing.T) {
	e := newEnv(t)
	e.login("a@example.com", "asifulla")
	e.login("b@example.com", "asif")
	e.login("c@example.com", "basil")

	// Typing part of a username finds it; an exact match comes first
	data := e.get("/api/v1/search/users?q=asi", "")
	require.ElementsMatch(t, []string{"asif", "asifulla"}, usernames(data["users"]))
	data = e.get("/api/v1/search/users?q="+url.QueryEscape("@ASIF"), "")
	require.Equal(t, []string{"asif", "asifulla"}, usernames(data["users"]))
	require.EqualValues(t, 2, data["pagination"].(map[string]interface{})["total"])

	data = e.get("/api/v1/search/combined?q=bas", "")
	require.Equal(t, []string{"basil"}, usernames(data["users"].(map[string]interface{})["items"]))
}

func TestBlockersHiddenFromSearch(t *testing.T) {
	e := newEnv(t)
	meTok, me := e.login("me@example.com", "searcher")
	blockerTok, blocker := e.login("blk@example.com", "gardener_one")
	friendTok, _ := e.login("fr@example.com", "gardener_two")
	e.create(blockerTok, "Gardening secrets")
	e.create(friendTok, "Gardening basics")
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": blocker}, bson.M{"$set": bson.M{"blockedUsers": []primitive.ObjectID{me}}})
	require.NoError(t, err)

	data := e.get("/api/v1/search/users?q=gardener", meTok)
	require.Equal(t, []string{"gardener_two"}, usernames(data["users"]))
	require.EqualValues(t, 1, data["pagination"].(map[string]interface{})["total"])

	data = e.get("/api/v1/search/anchors?q=gardening", meTok)
	require.Len(t, data["anchors"], 1)
	require.Equal(t, "Gardening basics", data["anchors"].([]interface{})[0].(map[string]interface{})["title"])
	require.EqualValues(t, 1, data["pagination"].(map[string]interface{})["total"])

	// Signed out, both are visible
	require.Len(t, e.get("/api/v1/search/anchors?q=gardening", "")["anchors"], 2)
}

func TestTagSuggestionsUseStoredTagFormat(t *testing.T) {
	e := newEnv(t)
	tok, _ := e.login("t@example.com", "tagger")
	e.create(tok, "One", "web-design")
	e.create(tok, "Two", "web-design", "webinars")
	e.create(tok, "Three", "weather")

	data := e.get("/api/v1/search/tags?q="+url.QueryEscape("#Web Des"), "")
	require.Equal(t, []interface{}{map[string]interface{}{"name": "web-design", "count": float64(2)}}, data["tags"])

	data = e.get("/api/v1/search/anchors?q=two&tag="+url.QueryEscape("#Webinars"), "")
	require.Len(t, data["anchors"], 1)
}

func TestSearchFailureIsNotAnEmptyResult(t *testing.T) {
	e := newEnv(t)
	e.login("x@example.com", "someone")
	_, err := e.db.Collection("users").Indexes().DropOne(context.Background(), "user_text_search")
	require.NoError(t, err)

	code, out := e.do("GET", "/api/v1/search?q=someone&type=users", "", nil)
	require.Equal(t, 500, code, "a broken search must not answer \"no users found\"")
	require.Equal(t, msgSearchFailed, out["message"])

	code, out = e.do("GET", "/api/v1/search?q=ab&type=anchors&limit=100", "", nil)
	require.Equal(t, 200, code, "a large limit is capped, not rejected: %v", out)
	code, out = e.do("GET", "/api/v1/search/anchors?q=ab&page=0", "", nil)
	require.Equal(t, 400, code)
	require.Equal(t, `Page must be a whole number of 1 or more (got "0")`, out["message"])
}

func anchorTitles(list interface{}) []string {
	var out []string
	for _, a := range list.([]interface{}) {
		out = append(out, a.(map[string]interface{})["title"].(string))
	}
	return out
}

func (e *env) createFull(token, title, description string, tagList ...string) string {
	code, out := e.do("POST", "/api/v1/anchors", token, map[string]interface{}{
		"title": title, "description": description, "visibility": "public", "tags": tagList})
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func TestAnchorSearchIsTypeAheadFriendly(t *testing.T) {
	e := newEnv(t)
	tok, _ := e.login("p@example.com", "planter")
	e.createFull(tok, "Gardening Tips", "for small balconies")
	e.createFull(tok, "Weekly notes", "some gardening ideas") // matches only in the description
	e.createFull(tok, "Café Crème recipes", "")
	e.createFull(tok, "Best pasta recipes", "")
	e.createFull(tok, "Cats of Rome", "")
	e.createFull(tok, "The Theory of Everything", "")

	search := func(q string) map[string]interface{} {
		return e.get("/api/v1/search/anchors?q="+url.QueryEscape(q), "")
	}

	// Half-typed words match, title matches rank above description matches
	require.Equal(t, []string{"Gardening Tips", "Weekly notes"}, anchorTitles(search("garde")["anchors"]))
	require.Equal(t, []string{"Gardening Tips"}, anchorTitles(search("garde tip")["anchors"]))
	require.Equal(t, []string{"Gardening Tips", "Weekly notes"}, anchorTitles(search("gardening t")["anchors"]), "a trailing letter being typed doesn't empty the results")

	// Accents, operators and common words
	require.Equal(t, []string{"Café Crème recipes"}, anchorTitles(search("cafe creme")["anchors"]))
	require.Equal(t, []string{"Cats of Rome"}, anchorTitles(search("-cats")["anchors"]), "'-' is not an exclusion")
	require.Equal(t, []string{"Best pasta recipes"}, anchorTitles(search("the best pasta")["anchors"]))
	require.Equal(t, []string{"The Theory of Everything"}, anchorTitles(search("the")["anchors"]), "a query of only common words still works")

	// Nothing has every word: show what matches some of them, and say so
	data := search("gardening xyzzy")
	require.Equal(t, []string{"Gardening Tips", "Weekly notes"}, anchorTitles(data["anchors"]))
	require.Equal(t, false, data["meta"].(map[string]interface{})["matchedAllWords"])
	require.Equal(t, true, search("garde")["meta"].(map[string]interface{})["matchedAllWords"])
}

func TestEditsAndRenamesAreSearchable(t *testing.T) {
	e := newEnv(t)
	tok, _ := e.login("q@example.com", "editor")
	id := e.createFull(tok, "Old title", "")

	code, out := e.do("PATCH", "/api/v1/anchors/"+id, tok, map[string]interface{}{"title": "Mountain trails", "tags": []string{"hiking"}})
	require.Equal(t, 200, code, out)
	require.Empty(t, e.get("/api/v1/search/anchors?q=old", "")["anchors"])
	require.Equal(t, []string{"Mountain trails"}, anchorTitles(e.get("/api/v1/search/anchors?q=hik", "")["anchors"]))

	code, out = e.do("PATCH", "/api/v1/users/me", tok, map[string]string{"displayName": "Priya Sharma"})
	require.Equal(t, 200, code, out)
	require.Equal(t, []string{"editor"}, usernames(e.get("/api/v1/search/users?q=shar", "")["users"]), "found by the start of a name word")
}

func TestBackfillIndexesExistingDocuments(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := primitive.NewObjectID()
	_, err := e.db.Collection("users").InsertOne(ctx, bson.M{"_id": owner, "username": "legacy_user", "displayName": "Old Timer",
		"email": "legacy@example.com", "googleId": "g-legacy"})
	require.NoError(t, err)
	_, err = e.db.Collection("anchors").InsertOne(ctx, bson.M{"userId": owner, "title": "Vintage bicycles", "visibility": "public", "createdAt": time.Now()})
	require.NoError(t, err)
	require.Empty(t, e.get("/api/v1/search/anchors?q=vint", "")["anchors"], "saved before search terms existed")

	require.NoError(t, NewRepository(e.db).Backfill(ctx))
	require.Equal(t, []string{"Vintage bicycles"}, anchorTitles(e.get("/api/v1/search/anchors?q=vint", "")["anchors"]))
	require.Equal(t, []string{"legacy_user"}, usernames(e.get("/api/v1/search/users?q=timer", "")["users"]))
}
