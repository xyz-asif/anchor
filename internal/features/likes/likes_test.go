package likes

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

// ---------- unit tests ----------

func TestLikeValidationMessages(t *testing.T) {
	req := &LikeActionRequest{Action: " LIKE "}
	require.NoError(t, ValidateLikeActionRequest(req))
	require.Equal(t, "like", req.Action)
	require.ErrorContains(t, ValidateLikeActionRequest(&LikeActionRequest{Action: "love"}), `"love" isn't a valid action`)

	_, err := ParseLikeListQuery("0", "")
	require.ErrorContains(t, err, `Page must be a whole number of 1 or more (got "0")`)
	q, err := ParseLikeListQuery("2", "999")
	require.NoError(t, err)
	require.Equal(t, maxListLimit, q.Limit)
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
	db := client.Database(fmt.Sprintf("likes_test_%d", time.Now().UnixNano()))
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

func (e *env) create(token, title, visibility string) string {
	code, out := e.do("POST", "/api/v1/anchors", token, map[string]string{"title": title, "visibility": visibility})
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func (e *env) like(token, anchorID, action string) (int, map[string]interface{}) {
	return e.do("POST", "/api/v1/anchors/"+anchorID+"/like", token, map[string]string{"action": action})
}

func (e *env) block(blocker, blocked primitive.ObjectID) {
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": blocker},
		bson.M{"$addToSet": bson.M{"blockedUsers": blocked}})
	require.NoError(e.t, err)
}

func (e *env) likeCount(anchorID string) int {
	id, _ := primitive.ObjectIDFromHex(anchorID)
	var doc struct {
		LikeCount int `bson:"likeCount"`
	}
	require.NoError(e.t, e.db.Collection("anchors").FindOne(context.Background(), bson.M{"_id": id}).Decode(&doc))
	return doc.LikeCount
}

func TestPrivateAndBlockedAnchorsLookNonExistent(t *testing.T) {
	e := newEnv(t)
	ownerTok, owner := e.login("owner@example.com")
	tok, me := e.login("me@example.com")
	private := e.create(ownerTok, "Private anchor", "private")
	public := e.create(ownerTok, "Public anchor", "public")

	code, _ := e.like(tok, private, "like")
	require.Equal(t, 404, code, "liking a private anchor must not reveal it exists")

	code, _ = e.like(tok, public, "like")
	require.Equal(t, 200, code)

	e.block(owner, me)
	for _, path := range []string{"/like/status", "/likes", "/like/summary"} {
		code, _ = e.do("GET", "/api/v1/anchors/"+public+path, tok, nil)
		require.Equal(t, 404, code, path)
	}
	code, _ = e.like(tok, public, "like")
	require.Equal(t, 404, code)

	// Removing an existing like always works
	code, out := e.like(tok, public, "unlike")
	require.Equal(t, 200, code, out)
	require.Equal(t, 0, e.likeCount(public))
}

func TestBlockingTheOwnerExplainsHowToLike(t *testing.T) {
	e := newEnv(t)
	ownerTok, owner := e.login("o2@example.com")
	tok, me := e.login("m2@example.com")
	a := e.create(ownerTok, "Some anchor", "public")
	e.block(me, owner)
	code, out := e.like(tok, a, "like")
	require.Equal(t, 403, code)
	require.Contains(t, out["message"], "Unblock them first")
}

func TestConcurrentLikesAndCounterNeverNegative(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("o3@example.com")
	tok, _ := e.login("m3@example.com")
	a := e.create(ownerTok, "Liked anchor", "public")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.like(tok, a, "like") }()
	}
	wg.Wait()
	require.Equal(t, 1, e.likeCount(a))

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.like(tok, a, "unlike") }()
	}
	wg.Wait()
	require.Equal(t, 0, e.likeCount(a))

	// A decrement on a zero count is a no-op instead of "-1 then fix"
	id, _ := primitive.ObjectIDFromHex(a)
	require.NoError(t, anchors.NewRepository(e.db).IncrementLikeCount(context.Background(), id, -1))
	require.Equal(t, 0, e.likeCount(a))
}

func TestLikersListAndSummaryRespectBlocksAndOrder(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("o4@example.com")
	viewerTok, viewer := e.login("v4@example.com")
	a := e.create(ownerTok, "Popular anchor", "public")

	// Likers in order: followed (oldest), x, y, blocker (newest)
	followedTok, followed := e.login("followed@example.com")
	xTok, _ := e.login("x@example.com")
	yTok, _ := e.login("y@example.com")
	blockerTok, blocker := e.login("blocker@example.com")
	for _, tok := range []string{followedTok, xTok, yTok, blockerTok} {
		code, _ := e.like(tok, a, "like")
		require.Equal(t, 200, code)
		time.Sleep(5 * time.Millisecond)
	}
	e.block(blocker, viewer)
	_, err := e.db.Collection("follows").InsertOne(context.Background(),
		bson.M{"followerId": viewer, "followingId": followed, "createdAt": time.Now()})
	require.NoError(t, err)

	code, out := e.do("GET", "/api/v1/anchors/"+a+"/likes", viewerTok, nil)
	require.Equal(t, 200, code, out)
	data := out["data"].(map[string]interface{})
	require.EqualValues(t, 3, data["pagination"].(map[string]interface{})["total"], "the user who blocked the viewer is not listed or counted")

	code, out = e.do("GET", "/api/v1/anchors/"+a+"/like/summary", viewerTok, nil)
	require.Equal(t, 200, code, out)
	shown := out["data"].(map[string]interface{})["likedByFollowing"].([]interface{})
	require.Len(t, shown, 3)
	require.Equal(t, followed.Hex(), shown[0].(map[string]interface{})["id"], "people the viewer follows come first")
	for _, u := range shown {
		require.NotEqual(t, blocker.Hex(), u.(map[string]interface{})["id"])
	}

	// No likes: lists are [] (not null)
	b := e.create(ownerTok, "Unliked anchor", "public")
	code, out = e.do("GET", "/api/v1/anchors/"+b+"/likes", "", nil)
	require.Equal(t, 200, code)
	require.NotNil(t, out["data"].(map[string]interface{})["data"])
}
