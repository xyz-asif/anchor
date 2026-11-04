package notifications_test

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
	"github.com/xyz-asif/gotodo/internal/features/anchor_follows"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestParseNotificationListQuery(t *testing.T) {
	_, err := notifications.ParseNotificationListQuery("", "", "yes")
	require.ErrorContains(t, err, `unreadOnly must be true or false (got "yes")`)
	q, err := notifications.ParseNotificationListQuery("2", "500", "TRUE")
	require.NoError(t, err)
	require.Equal(t, 2, q.Page)
	require.Equal(t, 50, q.Limit)
	require.True(t, q.UnreadOnly)
}

// Integration tests; they need AUTH_TEST_MONGO_URI (a throwaway local mongod).

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
	db := client.Database(fmt.Sprintf("notif_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		time.Sleep(100 * time.Millisecond)
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	cfg := &config.Config{AppEnv: "test", DevMode: true, JWTSecret: "test-secret-test-secret-test-secret-123",
		JWTExpireHours: 1, RefreshTokenExpireHours: 24}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")

	// Same wiring as internal/routes
	followsRepo := anchor_follows.NewRepository(db)
	service := notifications.GetService(db)
	service.SetFollowerProvider(followsRepo)
	auth.RegisterRoutes(api, db, cfg, nil, nil)
	anchors.RegisterRoutes(api, db, cfg, followsRepo, service)
	anchor_follows.RegisterRoutes(api, db, cfg)
	notifications.RegisterRoutes(api, db, cfg, anchors.NewRepository(db))
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

func (e *env) create(token, title string) string {
	code, out := e.do("POST", "/api/v1/anchors", token, map[string]string{"title": title, "visibility": "public"})
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func (e *env) addItem(token, anchorID string) {
	code, out := e.do("POST", "/api/v1/anchors/"+anchorID+"/items", token, map[string]string{"type": "text", "content": "new"})
	require.Equal(e.t, 201, code, out)
}

func (e *env) count(filter bson.M) int64 {
	n, err := e.db.Collection("notifications").CountDocuments(context.Background(), filter)
	require.NoError(e.t, err)
	return n
}

// eventually waits for async notification writes
func (e *env) eventually(filter bson.M, want int64) {
	require.Eventually(e.t, func() bool { return e.count(filter) == want }, 3*time.Second, 25*time.Millisecond,
		"expected %d notifications matching %v, have %d", want, filter, e.count(filter))
}

func TestAnchorUpdatesCollapseIntoOneUnreadNotification(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("owner@example.com")
	followerTok, follower := e.login("follower@example.com")
	_, muter := e.login("muter@example.com")
	a := e.create(ownerTok, "Followed anchor")
	for _, tok := range []string{followerTok} {
		code, out := e.do("POST", "/api/v1/anchors/"+a+"/follow", tok, map[string]interface{}{"action": "follow", "notifyOnUpdate": true})
		require.Equal(t, 200, code, out)
	}
	// A follower who muted the owner is skipped
	muterTok := func() string {
		code, out := e.do("POST", "/api/v1/auth/dev-login", "", map[string]string{"email": "muter@example.com"})
		require.Equal(t, 200, code)
		return out["data"].(map[string]interface{})["token"].(string)
	}()
	code, _ := e.do("POST", "/api/v1/anchors/"+a+"/follow", muterTok, map[string]interface{}{"action": "follow", "notifyOnUpdate": true})
	require.Equal(t, 200, code)
	var ownerDoc struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	require.NoError(t, e.db.Collection("users").FindOne(context.Background(), bson.M{"email": "owner@example.com"}).Decode(&ownerDoc))
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": muter}, bson.M{"$set": bson.M{"mutedUsers": []primitive.ObjectID{ownerDoc.ID}}})
	require.NoError(t, err)

	filter := bson.M{"recipientId": follower, "type": "anchor_update"}
	for i := 0; i < 3; i++ {
		e.addItem(ownerTok, a)
		time.Sleep(60 * time.Millisecond)
	}
	e.eventually(filter, 1)
	require.Zero(t, e.count(bson.M{"recipientId": muter}))

	// Once read, the next update is a new notification
	code, _ = e.do("PATCH", "/api/v1/notifications/read-all", followerTok, nil)
	require.Equal(t, 200, code)
	e.addItem(ownerTok, a)
	e.eventually(filter, 2)
}

func TestBlockedActorsHiddenAndCountsAgree(t *testing.T) {
	e := newEnv(t)
	meTok, me := e.login("me@example.com")
	_, friend := e.login("friend@example.com")
	_, troll := e.login("troll@example.com")
	ctx := context.Background()
	repo := notifications.NewRepository(e.db)
	for _, actor := range []primitive.ObjectID{friend, troll, troll} {
		require.NoError(t, repo.CreateNotification(ctx, &notifications.Notification{
			RecipientID: me, ActorID: actor, Type: notifications.TypeFollow, ResourceType: "user", ResourceID: actor,
		}))
	}
	_, err := e.db.Collection("users").UpdateOne(ctx, bson.M{"_id": me}, bson.M{"$set": bson.M{"blockedUsers": []primitive.ObjectID{troll}}})
	require.NoError(t, err)

	code, out := e.do("GET", "/api/v1/notifications", meTok, nil)
	require.Equal(t, 200, code, out)
	data := out["data"].(map[string]interface{})
	require.EqualValues(t, 1, data["pagination"].(map[string]interface{})["total"], "notifications from someone you blocked disappear")

	code, out = e.do("GET", "/api/v1/notifications/unread-count", meTok, nil)
	require.Equal(t, 200, code)
	require.EqualValues(t, 1, out["data"].(map[string]interface{})["unreadCount"], "the badge matches the list")
}

func TestOthersNotificationsLookNonExistent(t *testing.T) {
	e := newEnv(t)
	_, owner := e.login("o@example.com")
	otherTok, _ := e.login("x@example.com")
	n := &notifications.Notification{RecipientID: owner, ActorID: primitive.NewObjectID(), Type: notifications.TypeFollow, ResourceType: "user"}
	require.NoError(t, notifications.NewRepository(e.db).CreateNotification(context.Background(), n))

	code, out := e.do("PATCH", "/api/v1/notifications/"+n.ID.Hex()+"/read", otherTok, nil)
	require.Equal(t, 404, code, "someone else's notification must not answer 403")
	require.Equal(t, "This notification doesn't exist or was already deleted", out["message"])

	code, out = e.do("GET", "/api/v1/notifications?unreadOnly=yes", otherTok, nil)
	require.Equal(t, 400, code)
	require.Contains(t, out["message"], "unreadOnly must be true or false")
}

func TestRepeatedClonesNotifyOnce(t *testing.T) {
	e := newEnv(t)
	ownerTok, owner := e.login("o2@example.com")
	tok, _ := e.login("c2@example.com")
	a := e.create(ownerTok, "Clonable anchor")
	for i := 0; i < 2; i++ {
		code, out := e.do("POST", "/api/v1/anchors/"+a+"/clone", tok, nil)
		require.Equal(t, 201, code, out)
		clone := out["data"].(map[string]interface{})["id"].(string)
		time.Sleep(100 * time.Millisecond)
		code, _ = e.do("DELETE", "/api/v1/anchors/"+clone, tok, nil)
		require.Equal(t, 200, code)
	}
	time.Sleep(150 * time.Millisecond)
	require.EqualValues(t, 1, e.count(bson.M{"recipientId": owner, "type": "clone"}))
}
