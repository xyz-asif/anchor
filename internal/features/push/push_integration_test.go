package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// slowSender takes a while per push and fails once its context is done, like a real network call
type slowSender struct {
	mu    sync.Mutex
	delay time.Duration
	sent  int
}

func (s *slowSender) Send(ctx context.Context, msg Message) ([]string, error) {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s.mu.Lock()
	s.sent++
	s.mu.Unlock()
	return nil, nil
}

func TestBigBatchesAreAllSent(t *testing.T) {
	old := processTimeout
	processTimeout = 300 * time.Millisecond
	defer func() { processTimeout = old }()

	devices := map[primitive.ObjectID][]Device{}
	var batch []notifications.Notification
	for i := 0; i < 250; i++ { // an anchor update to 250 followers
		id := primitive.NewObjectID()
		devices[id] = []Device{{UserID: id, Token: fmt.Sprintf("token-%03d", i)}}
		batch = append(batch, notifications.Notification{RecipientID: id, Type: notifications.TypeAnchorUpdate})
	}
	sender := &slowSender{delay: 10 * time.Millisecond}
	d := NewDispatcher(&fakeStore{devices: devices}, fakeUsers{}, fakeUnread{}, sender)
	d.Dispatch(batch)
	d.Close()
	// One-by-one under a single time limit, 250 x 10ms is 2.5s: everything after 300ms was lost
	require.Equal(t, 250, sender.sent, "every follower gets the push")
}

func TestValidateDevice(t *testing.T) {
	req := RegisterDeviceRequest{Token: "  " + strings.Repeat("a", 40) + " ", Platform: " Android "}
	require.NoError(t, ValidateDevice(&req))
	require.Equal(t, "android", req.Platform)

	require.EqualError(t, ValidateDevice(&RegisterDeviceRequest{Platform: "ios"}), `Please send this device's push token as "token"`)
	require.EqualError(t, ValidateDevice(&RegisterDeviceRequest{Token: strings.Repeat("a", 40), Platform: "windows"}),
		`"windows" isn't a supported platform. Use "android", "ios" or "web"`)
	require.ErrorContains(t, ValidateDevice(&RegisterDeviceRequest{Token: "short", Platform: "ios"}), "doesn't look like a push token")
}

// ---------- integration tests (need AUTH_TEST_MONGO_URI) ----------

type env struct {
	t      *testing.T
	db     *mongo.Database
	router *gin.Engine
	repo   *Repository
}

func newEnv(t *testing.T) *env {
	uri := os.Getenv("AUTH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("AUTH_TEST_MONGO_URI not set; skipping integration test")
	}
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	require.NoError(t, err)
	db := client.Database(fmt.Sprintf("push_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		time.Sleep(50 * time.Millisecond)
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	cfg := &config.Config{AppEnv: "test", DevMode: true, JWTSecret: "test-secret-test-secret-test-secret-123",
		JWTExpireHours: 1, RefreshTokenExpireHours: 24, PushMode: "off"}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	auth.RegisterRoutes(api, db, cfg, nil, nil)
	RegisterRoutes(api, db, cfg)
	return &env{t: t, db: db, router: r, repo: NewRepository(db)}
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

// login returns access token, refresh token and user ID
func (e *env) login(email string) (string, string, primitive.ObjectID) {
	code, out := e.do("POST", "/api/v1/auth/dev-login", "", map[string]string{"email": email})
	require.Equal(e.t, 200, code, out)
	data := out["data"].(map[string]interface{})
	id, _ := primitive.ObjectIDFromHex(data["user"].(map[string]interface{})["id"].(string))
	return data["token"].(string), data["refreshToken"].(string), id
}

func (e *env) deviceTokens(user primitive.ObjectID) []string {
	devices, err := e.repo.DevicesForUsers(context.Background(), []primitive.ObjectID{user})
	require.NoError(e.t, err)
	var out []string
	for _, d := range devices[user] {
		out = append(out, d.Token)
	}
	return out
}

func TestLoggingOutStopsPushesToThatPhone(t *testing.T) {
	e := newEnv(t)
	phoneTok, phoneRefresh, me := e.login("me@example.com")
	tabletTok, _, _ := e.login("me@example.com")
	phone, tablet := "phone-token-"+strings.Repeat("p", 30), "tablet-token-"+strings.Repeat("t", 30)

	code, out := e.do("POST", "/api/v1/devices", phoneTok, map[string]string{"token": phone, "platform": "Android"})
	require.Equal(t, 200, code, out)
	code, out = e.do("POST", "/api/v1/devices", tabletTok, map[string]string{"token": tablet, "platform": "ios"})
	require.Equal(t, 200, code, out)
	require.ElementsMatch(t, []string{phone, tablet}, e.deviceTokens(me))

	// Logging out on the phone stops pushes there, but not on the tablet
	code, out = e.do("POST", "/api/v1/auth/logout", phoneTok, map[string]string{"refreshToken": phoneRefresh})
	require.Equal(t, 200, code, out)
	require.Equal(t, []string{tablet}, e.deviceTokens(me))

	// A session that ends without a logout (expired) stops pushes too
	_, err := e.db.Collection("refresh_tokens").UpdateMany(context.Background(), bson.M{"userId": me},
		bson.M{"$set": bson.M{"expiresAt": time.Now().Add(-time.Minute)}})
	require.NoError(t, err)
	require.Empty(t, e.deviceTokens(me))
}

func TestUnregisterIsSafeOnEveryLogout(t *testing.T) {
	e := newEnv(t)
	tok, _, _ := e.login("u@example.com")
	token := "device-token-" + strings.Repeat("x", 30)
	code, out := e.do("POST", "/api/v1/devices", tok, map[string]string{"token": token, "platform": "web"})
	require.Equal(t, 200, code, out)

	code, out = e.do("DELETE", "/api/v1/devices", tok, map[string]string{"token": token})
	require.Equal(t, 200, code, out)
	require.Equal(t, true, out["data"].(map[string]interface{})["removed"])

	// Again (e.g. the app retries): not an error
	code, out = e.do("DELETE", "/api/v1/devices", tok, map[string]string{"token": token})
	require.Equal(t, 200, code, out)
	require.Equal(t, false, out["data"].(map[string]interface{})["removed"])
}

func TestConcurrentSettingChangesAreAllKept(t *testing.T) {
	e := newEnv(t)
	tok, _, me := e.login("s@example.com")
	fields := []string{"likes", "comments", "replies", "mentions", "follows", "clones", "anchorUpdates"}

	var wg sync.WaitGroup
	for _, f := range fields {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()
			code, out := e.do("PATCH", "/api/v1/users/me/notification-settings", tok, map[string]bool{f: false})
			require.Equal(t, 200, code, out)
		}(f)
	}
	wg.Wait()

	s, err := e.repo.GetSettings(context.Background(), me)
	require.NoError(t, err)
	require.True(t, s.Enabled, "untouched switches keep their default")
	require.False(t, s.Likes || s.Comments || s.Replies || s.Mentions || s.Follows || s.Clones || s.AnchorUpdates,
		"7 quick toggles at once: none may be lost (%+v)", s)
}
