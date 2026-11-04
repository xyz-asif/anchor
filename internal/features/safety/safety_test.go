package safety

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
	"github.com/xyz-asif/gotodo/internal/features/anchor_follows"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/comments"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------- unit tests ----------

func TestReasonAndListQueryMessages(t *testing.T) {
	_, err := cleanReason("  ​  ")
	require.EqualError(t, err, "Please tell us what's wrong, so our team can review it")
	_, err = cleanReason("spam")
	require.EqualError(t, err, "Please describe the problem in at least 5 characters")
	_, err = cleanReason("  垃圾信息  ")
	require.EqualError(t, err, "Please describe the problem in at least 5 characters", "counted in characters: 4, not 12 bytes")
	r, err := cleanReason(" Spam links ‮ everywhere ")
	require.NoError(t, err)
	require.Equal(t, "Spam links  everywhere", r)

	_, err = parseReportListQuery("open", "", "", "")
	require.EqualError(t, err, `"open" isn't a report status. Use pending, reviewed, actioned or dismissed`)
	q, err := parseReportListQuery("PENDING", "Comment", "2", "500")
	require.NoError(t, err)
	require.Equal(t, &ReportListQuery{Status: "pending", TargetType: "comment", Page: 2, Limit: 100}, q)
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
	db := client.Database(fmt.Sprintf("safety_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		time.Sleep(50 * time.Millisecond)
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	cfg := &config.Config{AppEnv: "test", DevMode: true, JWTSecret: "test-secret-test-secret-test-secret-123",
		JWTExpireHours: 1, RefreshTokenExpireHours: 24, AdminEmails: []string{"admin@example.com"}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	auth.RegisterRoutes(api, db, cfg, nil, nil)
	anchors.RegisterRoutes(api, db, cfg, anchor_follows.NewRepository(db), notifications.GetService(db))
	anchor_follows.RegisterRoutes(api, db, cfg)
	comments.RegisterRoutes(api, db, cfg)
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

func (e *env) report(token, targetType, id, reason string) (int, map[string]interface{}) {
	return e.do("POST", "/api/v1/reports", token, map[string]string{"targetType": targetType, "targetId": id, "reason": reason})
}

func (e *env) count(coll string, filter bson.M) int64 {
	n, err := e.db.Collection(coll).CountDocuments(context.Background(), filter)
	require.NoError(e.t, err)
	return n
}

func TestReportsCantProbePrivateContent(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("owner@example.com")
	tok, _ := e.login("reporter@example.com")
	private := e.create(ownerTok, "Private anchor", "private")
	public := e.create(ownerTok, "Public anchor", "public")
	mine := e.create(tok, "My anchor", "public")

	code, out := e.report(tok, "anchor", private, "This looks like spam")
	require.Equal(t, 404, code, "someone else's private anchor must look non-existent")
	require.Equal(t, "This anchor doesn't exist or was already removed", out["message"])

	code, out = e.report(tok, "anchor", mine, "Reporting myself")
	require.Equal(t, 400, code)
	require.Equal(t, "You can't report your own anchor. You can edit or delete it instead", out["message"])

	// Comments can be reported now
	code, out = e.do("POST", "/api/v1/anchors/"+public+"/comments", ownerTok, map[string]string{"content": "rude words"})
	require.Equal(t, 201, code, out)
	commentID := out["data"].(map[string]interface{})["id"].(string)
	code, out = e.report(tok, "Comment", commentID, "Harassment in the comments")
	require.Equal(t, 201, code, out)

	code, out = e.report(tok, "post", public, "whatever this is")
	require.Equal(t, 400, code)
	require.Equal(t, `"post" can't be reported. targetType is anchor, item, comment or user`, out["message"])
}

func TestConcurrentDuplicateReportsBecomeOne(t *testing.T) {
	e := newEnv(t)
	ownerTok, owner := e.login("o2@example.com")
	tok, reporter := e.login("r2@example.com")
	a := e.create(ownerTok, "Reported anchor", "public")

	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = e.report(tok, "anchor", a, "Spam, reported in a hurry")
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 1, e.count("reports", bson.M{}), "double taps create one report: %v", codes)

	// The rate limit: 30 reports a day
	for i := 0; i < 29; i++ {
		_, err := e.db.Collection("reports").InsertOne(context.Background(), bson.M{
			"reporterId": reporter, "targetId": primitive.NewObjectID(), "createdAt": time.Now(), "open": false})
		require.NoError(t, err)
	}
	code, out := e.report(tok, "user", owner.Hex(), "Another report today")
	require.Equal(t, 429, code)
	require.Contains(t, out["message"], "30 reports in the last 24 hours")
}

func TestBlockRemovesAnchorFollowsAndCapsLists(t *testing.T) {
	e := newEnv(t)
	aTok, a := e.login("a@example.com")
	bTok, b := e.login("b@example.com")
	aAnchor := e.create(aTok, "A's anchor", "public")
	bAnchor := e.create(bTok, "B's anchor", "public")
	for _, f := range []struct{ tok, anchor string }{{aTok, bAnchor}, {bTok, aAnchor}} {
		code, out := e.do("POST", "/api/v1/anchors/"+f.anchor+"/follow", f.tok, map[string]interface{}{"action": "follow"})
		require.Equal(t, 200, code, out)
	}
	require.EqualValues(t, 2, e.count("anchor_follows", bson.M{}))

	code, out := e.do("POST", "/api/v1/users/"+b.Hex()+"/block", aTok, nil)
	require.Equal(t, 200, code, out)
	require.Equal(t, true, out["data"].(map[string]interface{})["isBlocked"])
	require.EqualValues(t, 0, e.count("anchor_follows", bson.M{}), "follows of each other's anchors end with the block")
	require.EqualValues(t, 0, e.count("anchors", bson.M{"followerCount": bson.M{"$gt": 0}}))

	// A block list at its maximum refuses new entries with an explanation
	full := make([]primitive.ObjectID, maxBlockedUsers)
	for i := range full {
		full[i] = primitive.NewObjectID()
	}
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": a}, bson.M{"$set": bson.M{"blockedUsers": full}})
	require.NoError(t, err)
	_, c := e.login("c@example.com")
	code, out = e.do("POST", "/api/v1/users/"+c.Hex()+"/block", aTok, nil)
	require.Equal(t, 400, code)
	require.Contains(t, out["message"], "which is the maximum")

	code, out = e.do("POST", "/api/v1/users/nope/block", aTok, nil)
	require.Equal(t, 400, code)
	require.Equal(t, `"nope" isn't a valid user ID`, out["message"])
	code, out = e.do("POST", "/api/v1/users/"+primitive.NewObjectID().Hex()+"/mute", aTok, nil)
	require.Equal(t, 404, code)
	require.Equal(t, msgUserNotFound, out["message"])
}

func TestHiddenAnchorsKeepNewest(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("o3@example.com")
	tok, me := e.login("h3@example.com")
	a := e.create(ownerTok, "Not for me", "public")
	private := e.create(ownerTok, "Private", "private")

	old := make([]primitive.ObjectID, maxHiddenAnchor)
	for i := range old {
		old[i] = primitive.NewObjectID()
	}
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": me}, bson.M{"$set": bson.M{"hiddenAnchors": old}})
	require.NoError(t, err)

	code, out := e.do("POST", "/api/v1/anchors/"+a+"/hide", tok, nil)
	require.Equal(t, 200, code, out)
	var u struct {
		Hidden []primitive.ObjectID `bson:"hiddenAnchors"`
	}
	require.NoError(t, e.db.Collection("users").FindOne(context.Background(), bson.M{"_id": me}).Decode(&u))
	require.Len(t, u.Hidden, maxHiddenAnchor, "the list doesn't grow past its maximum")
	require.Equal(t, a, u.Hidden[len(u.Hidden)-1].Hex(), "the newest entry is kept")

	code, _ = e.do("POST", "/api/v1/anchors/"+private+"/hide", tok, nil)
	require.Equal(t, 404, code, "someone else's private anchor looks non-existent")
}

func TestAdminReportFlow(t *testing.T) {
	e := newEnv(t)
	adminTok, _ := e.login("admin@example.com")
	tok, _ := e.login("r4@example.com")
	_, target := e.login("t4@example.com")

	code, out := e.report(tok, "user", target.Hex(), "Impersonating someone")
	require.Equal(t, 201, code, out)
	reportID := out["data"].(map[string]interface{})["id"].(string)

	code, out = e.do("PATCH", "/api/v1/admin/reports/"+reportID, adminTok, map[string]string{"status": "Dismissed"})
	require.Equal(t, 200, code, out)

	// Once closed, the same person can report again; reopening the old one then conflicts
	code, out = e.report(tok, "user", target.Hex(), "Still impersonating")
	require.Equal(t, 201, code, out)
	code, out = e.do("PATCH", "/api/v1/admin/reports/"+reportID, adminTok, map[string]string{"status": "pending"})
	require.Equal(t, 409, code, out)

	code, out = e.do("GET", "/api/v1/admin/reports?status=open", adminTok, nil)
	require.Equal(t, 400, code)
	require.Equal(t, `"open" isn't a report status. Use pending, reviewed, actioned or dismissed`, out["message"])

	code, out = e.do("PATCH", "/api/v1/admin/reports/"+reportID, adminTok, map[string]string{"status": "closed"})
	require.Equal(t, 400, code)
	require.True(t, strings.HasPrefix(out["message"].(string), `"closed" isn't a report status`))
}
