package comments

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
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------- unit tests ----------

func TestExtractMentions(t *testing.T) {
	require.Equal(t, []string{"alice", "bob_2"}, ExtractMentions("hi @Alice and @bob_2, also @alice again"))
	require.Empty(t, ExtractMentions("mail me@example.com or a.b@c.io"), "email addresses aren't mentions")
	require.Empty(t, ExtractMentions("@ab is too short, @"+strings.Repeat("x", 25)+" is too long"))
	require.Equal(t, []string{"abc", "def"}, ExtractMentions("(@abc)/@def!"))
	var many []string
	for i := 0; i < 15; i++ {
		many = append(many, fmt.Sprintf("@user%02d", i))
	}
	require.Len(t, ExtractMentions(strings.Join(many, " ")), maxMentions)
}

func TestCommentContentValidation(t *testing.T) {
	req := &CreateCommentRequest{Content: strings.Repeat("字", maxCommentLength)}
	require.NoError(t, ValidateCreateCommentRequest(req), "1,000 CJK characters (3,000 bytes) must be allowed")

	req = &CreateCommentRequest{Content: strings.Repeat("a", maxCommentLength+1)}
	require.ErrorContains(t, ValidateCreateCommentRequest(req), "at most 1000 characters long (this one has 1001)")

	req = &CreateCommentRequest{Content: " ​​ \n"}
	require.ErrorContains(t, ValidateCreateCommentRequest(req), "can't be empty")

	req = &CreateCommentRequest{Content: "  line one\nline two ‮ "}
	require.NoError(t, ValidateCreateCommentRequest(req))
	require.Equal(t, "line one\nline two", req.Content)

	_, err := ParseCommentListQuery("", "", "popular")
	require.ErrorContains(t, err, `"popular" isn't a valid sort`)
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
	db := client.Database(fmt.Sprintf("comments_test_%d", time.Now().UnixNano()))
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

// login returns token, user ID and username
func (e *env) login(email string) (string, primitive.ObjectID, string) {
	code, out := e.do("POST", "/api/v1/auth/dev-login", "", map[string]string{"email": email})
	require.Equal(e.t, 200, code, out)
	data := out["data"].(map[string]interface{})
	user := data["user"].(map[string]interface{})
	id, _ := primitive.ObjectIDFromHex(user["id"].(string))
	return data["token"].(string), id, user["username"].(string)
}

func (e *env) create(token, title, visibility string) string {
	code, out := e.do("POST", "/api/v1/anchors", token, map[string]string{"title": title, "visibility": visibility})
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func (e *env) comment(token, anchorID, content string) (int, map[string]interface{}) {
	return e.do("POST", "/api/v1/anchors/"+anchorID+"/comments", token, map[string]string{"content": content})
}

func (e *env) mustComment(token, anchorID, content string) string {
	code, out := e.comment(token, anchorID, content)
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func (e *env) reply(token, commentID, content string) string {
	code, out := e.do("POST", "/api/v1/comments/"+commentID+"/replies", token, map[string]string{"content": content})
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func (e *env) block(blocker, blocked primitive.ObjectID) {
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": blocker},
		bson.M{"$addToSet": bson.M{"blockedUsers": blocked}})
	require.NoError(e.t, err)
}

func (e *env) field(coll, hexID, field string) int {
	id, _ := primitive.ObjectIDFromHex(hexID)
	var doc bson.M
	require.NoError(e.t, e.db.Collection(coll).FindOne(context.Background(), bson.M{"_id": id}).Decode(&doc))
	switch v := doc[field].(type) {
	case int32:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}

func TestPrivateAnchorCommentsAndMentions(t *testing.T) {
	e := newEnv(t)
	ownerTok, _, _ := e.login("owner@example.com")
	otherTok, other, otherName := e.login("other@example.com")
	private := e.create(ownerTok, "Private anchor", "private")

	code, _ := e.comment(otherTok, private, "hello")
	require.Equal(t, 404, code, "a private anchor must look non-existent")

	// The owner mentions someone who can't see the anchor: no mention is stored, so no
	// notification with a preview of the private content is sent
	code, out := e.comment(ownerTok, private, "secret plans, right @"+otherName+"?")
	require.Equal(t, 201, code, out)
	require.Empty(t, out["data"].(map[string]interface{})["mentions"])
	time.Sleep(100 * time.Millisecond)
	n, err := e.db.Collection("notifications").CountDocuments(context.Background(), bson.M{"recipientId": other})
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestBlocksHideCommentsAndAnchors(t *testing.T) {
	e := newEnv(t)
	ownerTok, owner, _ := e.login("o2@example.com")
	viewerTok, viewer, _ := e.login("v2@example.com")
	blockerTok, blocker, _ := e.login("b2@example.com")
	a := e.create(ownerTok, "Busy anchor", "public")
	e.mustComment(ownerTok, a, "first")
	hiddenComment := e.mustComment(blockerTok, a, "from someone who blocks the viewer")
	e.block(blocker, viewer)

	code, out := e.do("GET", "/api/v1/anchors/"+a+"/comments", viewerTok, nil)
	require.Equal(t, 200, code, out)
	data := out["data"].(map[string]interface{})
	require.EqualValues(t, 1, data["pagination"].(map[string]interface{})["total"])
	require.Len(t, data["comments"], 1)

	code, _ = e.do("GET", "/api/v1/comments/"+hiddenComment, viewerTok, nil)
	require.Equal(t, 404, code)

	e.block(owner, viewer)
	code, _ = e.do("GET", "/api/v1/anchors/"+a+"/comments", viewerTok, nil)
	require.Equal(t, 404, code)
	code, _ = e.comment(viewerTok, a, "can I still comment?")
	require.Equal(t, 404, code)
}

func TestConcurrentCommentLikesAndDeletes(t *testing.T) {
	e := newEnv(t)
	ownerTok, _, _ := e.login("o3@example.com")
	tok, _, _ := e.login("u3@example.com")
	a := e.create(ownerTok, "Counted anchor", "public")
	top := e.mustComment(ownerTok, a, "top level")
	e.reply(tok, top, "reply one")
	e.reply(tok, top, "reply two")
	e.mustComment(tok, a, "another top level")
	require.Equal(t, 4, e.field("anchors", a, "commentCount"))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.do("POST", "/api/v1/comments/"+top+"/like", tok, map[string]string{"action": "like"})
		}()
	}
	wg.Wait()
	require.Equal(t, 1, e.field("comments", top, "likeCount"))

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.do("DELETE", "/api/v1/comments/"+top, ownerTok, nil) }()
	}
	wg.Wait()
	require.Equal(t, 1, e.field("anchors", a, "commentCount"), "the comment and its 2 replies are removed exactly once")
}

func TestEditAndDeleteRules(t *testing.T) {
	e := newEnv(t)
	ownerTok, _, _ := e.login("o4@example.com")
	tok, _, _ := e.login("u4@example.com")
	a := e.create(ownerTok, "Rules anchor", "public")
	mine := e.mustComment(tok, a, "my words")

	code, out := e.do("PATCH", "/api/v1/comments/"+mine, ownerTok, map[string]string{"content": "changed"})
	require.Equal(t, 403, code)
	require.Equal(t, "You can only edit your own comments", out["message"])

	code, out = e.do("PATCH", "/api/v1/comments/"+mine, tok, map[string]string{"content": "  my words "})
	require.Equal(t, 200, code, out)
	require.Equal(t, false, out["data"].(map[string]interface{})["isEdited"], "saving the same text isn't an edit")

	code, out = e.do("PATCH", "/api/v1/comments/"+mine, tok, map[string]string{})
	require.Equal(t, 400, code)
	require.Contains(t, out["message"], "can't be empty")

	// The anchor goes private: the author can still delete their own comment
	code, _ = e.do("PATCH", "/api/v1/anchors/"+a, ownerTok, map[string]string{"visibility": "private"})
	require.Equal(t, 200, code)
	code, out = e.do("DELETE", "/api/v1/comments/"+mine, tok, nil)
	require.Equal(t, 200, code, out)
}

func TestListRepliesOfAReplyExplains(t *testing.T) {
	e := newEnv(t)
	ownerTok, _, _ := e.login("o5@example.com")
	a := e.create(ownerTok, "Threaded anchor", "public")
	top := e.mustComment(ownerTok, a, "top")
	r := e.reply(ownerTok, top, "reply")

	code, out := e.do("GET", "/api/v1/comments/"+r+"/replies", "", nil)
	require.Equal(t, 400, code)
	require.Contains(t, out["message"], "/comments/"+top+"/replies")

	code, out = e.do("GET", "/api/v1/comments/"+top+"/replies?page=abc", "", nil)
	require.Equal(t, 400, code)
	require.Contains(t, out["message"], `Page must be a whole number`)
}
