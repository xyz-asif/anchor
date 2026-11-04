package follows

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
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------- unit tests ----------

func TestParseFollowListQueryMessages(t *testing.T) {
	_, err := ParseFollowListQuery("", "", "")
	require.ErrorContains(t, err, "?type=followers or ?type=following")

	_, err = ParseFollowListQuery("fans", "", "")
	require.ErrorContains(t, err, `"fans" isn't a valid list type`)

	_, err = ParseFollowListQuery("followers", "abc", "")
	require.ErrorContains(t, err, `Page must be a whole number of 1 or more (got "abc")`)

	_, err = ParseFollowListQuery("followers", "99999999999999999999", "")
	require.Error(t, err)

	q, err := ParseFollowListQuery("Followers", "2", "500")
	require.NoError(t, err)
	require.Equal(t, "followers", q.Type)
	require.Equal(t, 2, q.Page)
	require.Equal(t, maxListLimit, q.Limit, "too-large limits are clamped, not rejected")
}

func TestValidateFollowActionMessages(t *testing.T) {
	req := &FollowActionRequest{Action: " FOLLOW "}
	require.NoError(t, ValidateFollowActionRequest(req))
	require.Equal(t, "follow", req.Action)
	require.ErrorContains(t, ValidateFollowActionRequest(&FollowActionRequest{Action: "subscribe"}), `"subscribe" isn't a valid action`)
	require.ErrorContains(t, ValidateFollowActionRequest(&FollowActionRequest{}), `{"action": "follow"}`)
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
	db := client.Database(fmt.Sprintf("follows_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		time.Sleep(50 * time.Millisecond) // let async notification writes finish before dropping
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

func (e *env) login(email string) (string, primitive.ObjectID) {
	code, out := e.do("POST", "/api/v1/auth/dev-login", "", map[string]string{"email": email})
	require.Equal(e.t, 200, code, out)
	data := out["data"].(map[string]interface{})
	id, _ := primitive.ObjectIDFromHex(data["user"].(map[string]interface{})["id"].(string))
	return data["token"].(string), id
}

func (e *env) follow(token string, target primitive.ObjectID, action string) (int, map[string]interface{}) {
	return e.do("POST", "/api/v1/users/"+target.Hex()+"/follow", token, map[string]string{"action": action})
}

func (e *env) counts(id primitive.ObjectID) (followers, following int32) {
	var u struct {
		FollowerCount  int32 `bson:"followerCount"`
		FollowingCount int32 `bson:"followingCount"`
	}
	require.NoError(e.t, e.db.Collection("users").FindOne(context.Background(), bson.M{"_id": id}).Decode(&u))
	return u.FollowerCount, u.FollowingCount
}

func (e *env) block(blocker, blocked primitive.ObjectID) {
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": blocker},
		bson.M{"$addToSet": bson.M{"blockedUsers": blocked}})
	require.NoError(e.t, err)
}

func TestConcurrentFollowsCountOnce(t *testing.T) {
	e := newEnv(t)
	tok, me := e.login("a@example.com")
	_, target := e.login("b@example.com")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, out := e.follow(tok, target, "follow")
			require.Equal(t, 200, code, out)
		}()
	}
	wg.Wait()
	followers, _ := e.counts(target)
	_, following := e.counts(me)
	require.EqualValues(t, 1, followers)
	require.EqualValues(t, 1, following)

	for i := 0; i < 3; i++ {
		code, _ := e.follow(tok, target, "unfollow")
		require.Equal(t, 200, code)
	}
	followers, _ = e.counts(target)
	require.EqualValues(t, 0, followers, "repeated unfollows never go below zero")
}

func TestFollowErrorsAreSpecific(t *testing.T) {
	e := newEnv(t)
	tok, me := e.login("c@example.com")

	code, out := e.follow(tok, me, "follow")
	require.Equal(t, 400, code)
	require.Equal(t, "You can't follow or unfollow yourself", out["message"])

	code, out = e.do("POST", "/api/v1/users/nope/follow", tok, map[string]string{"action": "follow"})
	require.Equal(t, 400, code)
	require.Equal(t, `"nope" isn't a valid user ID`, out["message"])

	code, out = e.follow(tok, primitive.NewObjectID(), "follow")
	require.Equal(t, 404, code)
	require.Contains(t, out["message"], "doesn't exist")

	code, out = e.do("GET", "/api/v1/users/"+me.Hex()+"/follows", "", nil)
	require.Equal(t, 400, code)
	require.Contains(t, out["message"], "?type=followers")
}

func TestBlocksAndFollowing(t *testing.T) {
	e := newEnv(t)
	tok, me := e.login("d@example.com")
	_, blockedByMe := e.login("e@example.com")
	_, blocksMe := e.login("f@example.com")
	e.block(me, blockedByMe)
	e.block(blocksMe, me)

	code, out := e.follow(tok, blockedByMe, "follow")
	require.Equal(t, 403, code)
	require.Contains(t, out["message"], "Unblock them first")

	code, _ = e.follow(tok, blocksMe, "follow")
	require.Equal(t, 404, code, "a user who blocked you looks like they don't exist")

	code, _ = e.follow(tok, blocksMe, "unfollow")
	require.Equal(t, 200, code, "unfollowing must always be possible")

	code, _ = e.do("GET", "/api/v1/users/"+blocksMe.Hex()+"/follow/status", tok, nil)
	require.Equal(t, 404, code)
}

func TestFollowListsHideBlockedAndReturnEmptyArray(t *testing.T) {
	e := newEnv(t)
	viewerTok, viewer := e.login("g@example.com")
	_, popular := e.login("h@example.com")
	tokX, x := e.login("x@example.com")
	tokY, _ := e.login("y@example.com")
	tokZ, _ := e.login("z@example.com")
	for _, tok := range []string{tokX, tokY, tokZ} {
		code, _ := e.follow(tok, popular, "follow")
		require.Equal(t, 200, code)
	}
	e.block(x, viewer) // x blocked the viewer: x is left out of what the viewer sees

	code, out := e.do("GET", "/api/v1/users/"+popular.Hex()+"/follows?type=followers&limit=1", viewerTok, nil)
	require.Equal(t, 200, code, out)
	data := out["data"].(map[string]interface{})
	require.EqualValues(t, 2, data["pagination"].(map[string]interface{})["total"])
	require.Len(t, data["data"], 1)

	code, out = e.do("GET", "/api/v1/users/"+popular.Hex()+"/follows?type=followers", "", nil)
	require.Equal(t, 200, code)
	require.EqualValues(t, 3, out["data"].(map[string]interface{})["pagination"].(map[string]interface{})["total"])

	code, out = e.do("GET", "/api/v1/users/me/follows?type=following", viewerTok, nil)
	require.Equal(t, 200, code)
	require.NotNil(t, out["data"].(map[string]interface{})["data"], "an empty list is [], not null")
}
