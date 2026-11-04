package anchor_follows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"sync"
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

// Integration tests; they need AUTH_TEST_MONGO_URI (a throwaway local mongod).

type env struct {
	t      *testing.T
	db     *mongo.Database
	repo   *Repository
	router *gin.Engine
}

func newEnv(t *testing.T) *env {
	uri := os.Getenv("AUTH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("AUTH_TEST_MONGO_URI not set; skipping integration test")
	}
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	require.NoError(t, err)
	db := client.Database(fmt.Sprintf("afollows_test_%d", time.Now().UnixNano()))
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
	repo := NewRepository(db)
	auth.RegisterRoutes(api, db, cfg, nil, nil)
	anchors.RegisterRoutes(api, db, cfg, repo, notifications.GetService(db))
	RegisterRoutes(api, db, cfg)
	return &env{t: t, db: db, repo: repo, router: r}
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

func (e *env) create(token, title, visibility string) string {
	code, out := e.do("POST", "/api/v1/anchors", token, map[string]string{"title": title, "visibility": visibility})
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func (e *env) follow(token, anchorID, action string, notify *bool) (int, map[string]interface{}) {
	body := map[string]interface{}{"action": action}
	if notify != nil {
		body["notifyOnUpdate"] = *notify
	}
	return e.do("POST", "/api/v1/anchors/"+anchorID+"/follow", token, body)
}

func (e *env) block(blocker, blocked primitive.ObjectID) {
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": blocker},
		bson.M{"$addToSet": bson.M{"blockedUsers": blocked}})
	require.NoError(e.t, err)
}

func listData(out map[string]interface{}) ([]interface{}, map[string]interface{}, map[string]interface{}) {
	d := out["data"].(map[string]interface{})
	return d["data"].([]interface{}), d["pagination"].(map[string]interface{}), d["meta"].(map[string]interface{})
}

func TestToggleNotificationsRequiresValue(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("o@example.com")
	tok, me := e.login("f@example.com")
	a := e.create(ownerTok, "Followed anchor", "public")
	yes := true
	code, _ := e.follow(tok, a, "follow", &yes)
	require.Equal(t, 200, code)

	code, out := e.do("PATCH", "/api/v1/anchors/"+a+"/follow/notifications", tok, map[string]interface{}{})
	require.Equal(t, 400, code, "a missing value must not silently turn notifications off")
	require.Contains(t, out["message"], "notifyOnUpdate")

	f, err := e.repo.GetFollow(context.Background(), me, mustAnchor(t, a))
	require.NoError(t, err)
	require.True(t, f.NotifyOnUpdate, "the setting must be unchanged")
}

func mustAnchor(t *testing.T, hex string) primitive.ObjectID {
	id, err := primitive.ObjectIDFromHex(hex)
	require.NoError(t, err)
	return id
}

func TestFollowRulesAndUnfollowAlwaysWorks(t *testing.T) {
	e := newEnv(t)
	ownerTok, owner := e.login("owner@example.com")
	tok, me := e.login("me@example.com")
	private := e.create(ownerTok, "Private anchor", "private")
	public := e.create(ownerTok, "Public anchor", "public")

	code, _ := e.follow(tok, private, "follow", nil)
	require.Equal(t, 404, code, "a private anchor must look non-existent, not 403")

	code, out := e.follow(tok, public, "subscribe", nil)
	require.Equal(t, 400, code)
	require.Contains(t, out["message"], `"subscribe" isn't a valid action`)

	code, _ = e.follow(tok, public, "follow", nil)
	require.Equal(t, 200, code)

	// The owner blocks me and makes the anchor private: I can no longer follow or see it ...
	e.block(owner, me)
	code, _ = e.do("GET", "/api/v1/anchors/"+public+"/follow/status", tok, nil)
	require.Equal(t, 404, code)
	// ... but I can always unfollow
	code, out = e.follow(tok, public, "unfollow", nil)
	require.Equal(t, 200, code, out)
	require.Equal(t, false, out["data"].(map[string]interface{})["isFollowing"])
}

func TestConcurrentFollowCountsOnce(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("o2@example.com")
	tok, _ := e.login("f2@example.com")
	a := e.create(ownerTok, "Popular anchor", "public")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.follow(tok, a, "follow", nil) }()
	}
	wg.Wait()
	var doc struct {
		FollowerCount int `bson:"followerCount"`
	}
	require.NoError(t, e.db.Collection("anchors").FindOne(context.Background(), bson.M{"_id": mustAnchor(t, a)}).Decode(&doc))
	require.Equal(t, 1, doc.FollowerCount)
}

func TestFollowingListFiltersInQueryAndSorts(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("o3@example.com")
	blockerTok, blocker := e.login("b3@example.com")
	tok, me := e.login("f3@example.com")

	titles := []string{"Charlie", "alpha", "Bravo"}
	var ids []string
	for _, title := range titles {
		id := e.create(ownerTok, title, "public")
		ids = append(ids, id)
		code, _ := e.follow(tok, id, "follow", nil)
		require.Equal(t, 200, code)
	}
	hidden := e.create(blockerTok, "Blocker anchor", "public")
	code, _ := e.follow(tok, hidden, "follow", nil)
	require.Equal(t, 200, code)
	goesPrivate := e.create(ownerTok, "Goes private", "public")
	code, _ = e.follow(tok, goesPrivate, "follow", nil)
	require.Equal(t, 200, code)

	e.block(blocker, me)
	code, _ = e.do("PATCH", "/api/v1/anchors/"+goesPrivate, ownerTok, map[string]string{"visibility": "private"})
	require.Equal(t, 200, code)
	// One anchor gets an update the follower hasn't seen
	code, _ = e.do("POST", "/api/v1/anchors/"+ids[2]+"/items", ownerTok, map[string]string{"type": "text", "content": "new"})
	require.Equal(t, 201, code)

	code, out := e.do("GET", "/api/v1/users/me/following-anchors?sort=alphabetical&limit=2", tok, nil)
	require.Equal(t, 200, code, out)
	items, p, meta := listData(out)
	require.EqualValues(t, 3, p["total"], "blocked owner's and now-private anchors are excluded from the total")
	require.Len(t, items, 2)
	require.Equal(t, "alpha", items[0].(map[string]interface{})["title"])
	require.Equal(t, "Bravo", items[1].(map[string]interface{})["title"])
	require.EqualValues(t, 1, meta["totalWithUpdates"])

	code, out = e.do("GET", "/api/v1/users/me/following-anchors?hasUpdates=true", tok, nil)
	require.Equal(t, 200, code)
	items, p, _ = listData(out)
	require.EqualValues(t, 1, p["total"])
	require.Equal(t, "Bravo", items[0].(map[string]interface{})["title"])

	code, out = e.do("GET", "/api/v1/users/me/following-anchors?sort=newest", tok, nil)
	require.Equal(t, 400, code)
	require.Contains(t, out["message"], `"newest" isn't a valid sort`)
}

func TestLastSeenVersionNeverGoesBack(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("o4@example.com")
	tok, me := e.login("f4@example.com")
	a := e.create(ownerTok, "Versioned", "public")
	code, _ := e.follow(tok, a, "follow", nil)
	require.Equal(t, 200, code)
	ctx := context.Background()
	require.NoError(t, e.repo.UpdateLastSeenVersion(ctx, me, mustAnchor(t, a), 5))
	require.NoError(t, e.repo.UpdateLastSeenVersion(ctx, me, mustAnchor(t, a), 3)) // late, older view
	f, err := e.repo.GetFollow(ctx, me, mustAnchor(t, a))
	require.NoError(t, err)
	require.Equal(t, 5, f.LastSeenVersion)
}

func TestBlockedFollowersGetNoUpdateNotifications(t *testing.T) {
	e := newEnv(t)
	ownerTok, owner := e.login("o5@example.com")
	tokA, a := e.login("a5@example.com")
	tokB, b := e.login("b5@example.com")
	anchorID := e.create(ownerTok, "Notifying anchor", "public")
	yes := true
	for _, tok := range []string{tokA, tokB} {
		code, _ := e.follow(tok, anchorID, "follow", &yes)
		require.Equal(t, 200, code)
	}
	e.block(owner, b)

	ids, err := e.repo.GetFollowersWithNotifications(context.Background(), mustAnchor(t, anchorID))
	require.NoError(t, err)
	require.Equal(t, []primitive.ObjectID{a}, ids)
}
