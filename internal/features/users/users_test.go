package users

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Integration tests; they need AUTH_TEST_MONGO_URI (e.g. a throwaway local mongod).

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
	db := client.Database(fmt.Sprintf("users_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	cfg := &config.Config{AppEnv: "test", DevMode: true, JWTSecret: "test-secret-test-secret-test-secret-123",
		JWTExpireHours: 1, RefreshTokenExpireHours: 24}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	RegisterRoutes(api, db, cfg)
	auth.RegisterRoutes(api, db, cfg, nil, nil)
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

// login returns an access token, the user ID and the username
func (e *env) login(email string) (string, primitive.ObjectID, string) {
	code, out := e.do("POST", "/api/v1/auth/dev-login", "", map[string]string{"email": email})
	require.Equal(e.t, 200, code, out)
	data := out["data"].(map[string]interface{})
	user := data["user"].(map[string]interface{})
	id, _ := primitive.ObjectIDFromHex(user["id"].(string))
	return data["token"].(string), id, user["username"].(string)
}

func (e *env) anchor(owner primitive.ObjectID, visibility string, deleted bool, clonedFrom *primitive.ObjectID) primitive.ObjectID {
	doc := bson.M{"userId": owner, "title": visibility, "visibility": visibility, "createdAt": time.Now()}
	if deleted {
		doc["deletedAt"] = time.Now()
	}
	if clonedFrom != nil {
		doc["clonedFromAnchorId"] = *clonedFrom
	}
	res, err := e.db.Collection("anchors").InsertOne(context.Background(), doc)
	require.NoError(e.t, err)
	return res.InsertedID.(primitive.ObjectID)
}

func (e *env) like(user, anchor primitive.ObjectID, at time.Time) {
	_, err := e.db.Collection("likes").InsertOne(context.Background(), bson.M{"userId": user, "anchorId": anchor, "createdAt": at})
	require.NoError(e.t, err)
}

func (e *env) block(blocker, blocked primitive.ObjectID) {
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": blocker},
		bson.M{"$addToSet": bson.M{"blockedUsers": blocked}})
	require.NoError(e.t, err)
}

func pageOf(out map[string]interface{}) ([]interface{}, map[string]interface{}) {
	data := out["data"].(map[string]interface{})
	return data["data"].([]interface{}), data["pagination"].(map[string]interface{})
}

func TestLikesListOnlyCountsVisibleAnchors(t *testing.T) {
	e := newEnv(t)
	_, likerID, _ := e.login("liker@example.com")
	viewerTok, viewerID, _ := e.login("viewer@example.com")
	_, authorID, _ := e.login("author@example.com")
	_, blockerID, _ := e.login("blocker@example.com")

	now := time.Now()
	// 3 visible likes, interleaved with 5 hidden ones (newest first)
	e.like(likerID, e.anchor(authorID, "public", false, nil), now.Add(-1*time.Minute))
	e.like(likerID, e.anchor(authorID, "private", false, nil), now.Add(-2*time.Minute))
	e.like(likerID, e.anchor(authorID, "unlisted", false, nil), now.Add(-3*time.Minute))
	e.like(likerID, e.anchor(authorID, "public", true, nil), now.Add(-4*time.Minute))
	e.like(likerID, e.anchor(blockerID, "public", false, nil), now.Add(-5*time.Minute))
	e.like(likerID, e.anchor(authorID, "public", false, nil), now.Add(-6*time.Minute))
	e.like(likerID, e.anchor(viewerID, "private", false, nil), now.Add(-7*time.Minute)) // viewer's own
	e.like(likerID, e.anchor(authorID, "private", false, nil), now.Add(-8*time.Minute))
	e.block(blockerID, viewerID)

	code, out := e.do("GET", "/api/v1/users/"+likerID.Hex()+"/likes?limit=2", viewerTok, nil)
	require.Equal(t, 200, code, out)
	items, p := pageOf(out)
	require.Len(t, items, 2, "first page must be full")
	require.EqualValues(t, 3, p["total"], "total must not count hidden anchors")
	require.Equal(t, true, p["hasMore"])

	code, out = e.do("GET", "/api/v1/users/"+likerID.Hex()+"/likes?limit=2&page=2", viewerTok, nil)
	require.Equal(t, 200, code)
	items, p = pageOf(out)
	require.Len(t, items, 1)
	require.Equal(t, false, p["hasMore"])

	// Anonymous: the 3 public, non-deleted ones (no block relationship applies)
	code, out = e.do("GET", "/api/v1/users/"+likerID.Hex()+"/likes", "", nil)
	require.Equal(t, 200, code)
	_, p = pageOf(out)
	require.EqualValues(t, 3, p["total"])
}

func TestBlockedViewerCannotSeeProfileOrLists(t *testing.T) {
	e := newEnv(t)
	viewerTok, viewerID, _ := e.login("v2@example.com")
	_, targetID, targetName := e.login("t2@example.com")
	e.block(targetID, viewerID)

	for _, path := range []string{
		"/api/v1/users/username/" + targetName,
		"/api/v1/users/" + targetID.Hex() + "/likes",
		"/api/v1/users/" + targetID.Hex() + "/clones",
	} {
		code, _ := e.do("GET", path, viewerTok, nil)
		require.Equal(t, 404, code, path)
		code, _ = e.do("GET", path, "", nil)
		require.Equal(t, 200, code, "anonymous: "+path)
	}
}

func TestClonesHideNonPublicOriginals(t *testing.T) {
	e := newEnv(t)
	_, clonerID, _ := e.login("cloner@example.com")
	_, authorID, _ := e.login("orig@example.com")

	public := e.anchor(authorID, "public", false, nil)
	unlisted := e.anchor(authorID, "unlisted", false, nil)
	e.anchor(clonerID, "public", false, &public)
	e.anchor(clonerID, "public", false, &unlisted)

	code, out := e.do("GET", "/api/v1/users/"+clonerID.Hex()+"/clones", "", nil)
	require.Equal(t, 200, code, out)
	items, _ := pageOf(out)
	require.Len(t, items, 2)
	shown := map[string]bool{}
	for _, it := range items {
		m := it.(map[string]interface{})
		if id, ok := m["clonedFromAnchorId"].(string); ok {
			shown[id] = true
			require.NotNil(t, m["originalAuthor"])
		} else {
			require.Nil(t, m["originalAuthor"])
		}
	}
	require.True(t, shown[public.Hex()])
	require.False(t, shown[unlisted.Hex()], "an unlisted anchor's ID is its secret link")
}

func TestHugePageDoesNotError(t *testing.T) {
	e := newEnv(t)
	_, id, _ := e.login("p@example.com")
	code, _ := e.do("GET", "/api/v1/users/"+id.Hex()+"/likes?page=9223372036854775807&limit=50", "", nil)
	require.Equal(t, 200, code)
}
