package anchors

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

// ---------- unit tests ----------

func TestValidateCover(t *testing.T) {
	require.NoError(t, ValidateCover("", ""))
	require.NoError(t, ValidateCover("emoji", "🎵"))
	require.NoError(t, ValidateCover("icon", "music-note"))
	require.NoError(t, ValidateCover("image", "https://res.cloudinary.com/x/a.png"))
	require.ErrorContains(t, ValidateCover("image", "javascript:alert(1)"), "http:// or https://")
	require.ErrorContains(t, ValidateCover("", "🎵"), "coverMediaType")
	require.ErrorContains(t, ValidateCover("emoji", strings.Repeat("😀", 40)), "single emoji")
	require.Error(t, ValidateCover("icon", "a\nb"))
}

func TestNormalizeAndValidateTags(t *testing.T) {
	got, err := NormalizeAndValidateTags([]string{"#Go", "go", " Web Dev "})
	require.NoError(t, err)
	require.Equal(t, []string{"go", "web-dev"}, got)

	_, err = NormalizeAndValidateTags([]string{"ok-tag", "!!"})
	require.ErrorContains(t, err, `"!!" can't be used as a tag`)

	_, err = NormalizeAndValidateTags([]string{"aa", "bb", "cc", "dd", "ee", "ff"})
	require.ErrorContains(t, err, "at most 5 tags")
}

func TestValidateTextContentCountsCharacters(t *testing.T) {
	require.NoError(t, ValidateTextContent(strings.Repeat("字", maxTextContent)), "10,000 CJK characters are 30,000 bytes but must be allowed")
	require.ErrorContains(t, ValidateTextContent(strings.Repeat("a", maxTextContent+1)), "at most 10000")
	require.Error(t, ValidateTextContent("   "))
}

func TestValidateURL(t *testing.T) {
	require.NoError(t, ValidateURL("HTTPS://Example.com/a"))
	require.Error(t, ValidateURL("javascript:alert(1)"))
	require.Error(t, ValidateURL("https://"))
	require.ErrorContains(t, ValidateURL("https://e.com/"+strings.Repeat("a", maxURLLength)), "too long")
}

func TestScraperDropsUnsafeLinksAndLongText(t *testing.T) {
	page := `<html><head><title>` + strings.Repeat("t", 5000) + `</title>
		<meta property="og:image" content="javascript:alert(1)">
		<link rel="icon" href="/favicon.ico"></head></html>`
	data := parseHTMLMetadata(strings.NewReader(page), "https://final.example.com/path")
	require.Empty(t, data.Thumbnail, "javascript: thumbnails must be dropped")
	require.Equal(t, "https://final.example.com/favicon.ico", data.Favicon)
	require.LessOrEqual(t, len([]rune(data.Title)), maxScrapedTitle+1)
}

func TestCleanFilename(t *testing.T) {
	require.Equal(t, "report.pdf", cleanFilename(`C:\Users\me\report.pdf`, ".pdf"))
	require.Equal(t, "report.pdf", cleanFilename("../../etc/report.pdf", ".pdf"))
	require.Equal(t, "evil.pdf", cleanFilename("evil\u202e.pdf", ".pdf"))
	require.Equal(t, "file.pdf", cleanFilename("", ".pdf"))
	long := cleanFilename(strings.Repeat("a", 500)+".pdf", ".pdf")
	require.Equal(t, maxFilenameLength, len([]rune(long)))
	require.True(t, strings.HasSuffix(long, ".pdf"))
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
	db := client.Database(fmt.Sprintf("anchors_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		time.Sleep(50 * time.Millisecond) // let async notification writes finish
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	cfg := &config.Config{AppEnv: "test", DevMode: true, JWTSecret: "test-secret-test-secret-test-secret-123",
		JWTExpireHours: 1, RefreshTokenExpireHours: 24}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	auth.RegisterRoutes(api, db, cfg, nil, nil)
	RegisterRoutes(api, db, cfg, nil, notifications.GetService(db))
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

func (e *env) addText(token, anchorID, text string) string {
	code, out := e.do("POST", "/api/v1/anchors/"+anchorID+"/items", token, map[string]string{"type": "text", "content": text})
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func data(out map[string]interface{}) map[string]interface{} {
	return out["data"].(map[string]interface{})
}

func TestCreateValidationMessages(t *testing.T) {
	e := newEnv(t)
	tok, _ := e.login("a@example.com")

	cases := []struct {
		body interface{}
		msg  string
	}{
		{map[string]string{"title": "      ab"}, "at least 3 characters"},
		{map[string]string{"title": strings.Repeat("x", 101)}, `"title" can have at most 100 characters`},
		{map[string]interface{}{"title": 42}, `"title" must be text, not a number`},
		{map[string]string{"title": "Good title", "visibility": "friends"}, `"visibility" must be one of: private, unlisted, public`},
		{map[string]string{"title": "Good title", "coverMediaType": "image", "coverMediaValue": "javascript:alert(1)"}, "http:// or https://"},
		{map[string]interface{}{"title": "Good title", "tags": []string{"fine", "@@"}}, `"@@" can't be used as a tag`},
	}
	for _, tc := range cases {
		code, out := e.do("POST", "/api/v1/anchors", tok, tc.body)
		require.Equal(t, 400, code, tc.body)
		require.Contains(t, out["message"], tc.msg)
	}

	code, out := e.do("POST", "/api/v1/anchors", tok, map[string]string{"title": "  Trimmed title  "})
	require.Equal(t, 201, code)
	require.Equal(t, "Trimmed title", data(out)["title"])
}

func TestBlockedViewerCannotSeeOwnersAnchors(t *testing.T) {
	e := newEnv(t)
	ownerTok, owner := e.login("owner@example.com")
	viewerTok, viewer := e.login("viewer@example.com")
	anchorID := e.create(ownerTok, "Owner anchor", "public")
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": owner},
		bson.M{"$set": bson.M{"blockedUsers": []primitive.ObjectID{viewer}}})
	require.NoError(t, err)

	for _, path := range []string{"/api/v1/anchors/" + anchorID, "/api/v1/anchors/" + anchorID + "/items", "/api/v1/anchors?userId=" + owner.Hex()} {
		code, _ := e.do("GET", path, viewerTok, nil)
		require.Equal(t, 404, code, path)
		code, _ = e.do("GET", path, "", nil)
		require.Equal(t, 200, code, "anonymous: "+path)
	}
}

func TestCloneSourceHiddenWhenOriginalNoLongerPublic(t *testing.T) {
	e := newEnv(t)
	aTok, _ := e.login("orig@example.com")
	bTok, _ := e.login("cloner@example.com")
	cTok, _ := e.login("other@example.com")
	original := e.create(aTok, "Original anchor", "public")

	code, out := e.do("POST", "/api/v1/anchors/"+original+"/clone", bTok, nil)
	require.Equal(t, 201, code, out)
	clone := data(out)["id"].(string)

	code, _ = e.do("PATCH", "/api/v1/anchors/"+original, aTok, map[string]string{"visibility": "unlisted"})
	require.Equal(t, 200, code)

	code, out = e.do("GET", "/api/v1/anchors/"+clone, cTok, nil)
	require.Equal(t, 200, code)
	require.Nil(t, data(out)["anchor"].(map[string]interface{})["clonedFromAnchorId"], "the unlisted original's ID (its secret link) must not leak")

	code, out = e.do("GET", "/api/v1/anchors/"+clone, bTok, nil)
	require.Equal(t, 200, code)
	require.Equal(t, original, data(out)["anchor"].(map[string]interface{})["clonedFromAnchorId"], "the clone's owner still sees it")
}

func TestConcurrentDeleteAdjustsCountOnce(t *testing.T) {
	e := newEnv(t)
	tok, me := e.login("del@example.com")
	e.create(tok, "Keep one", "public")
	e.create(tok, "Keep two", "public")
	target := e.create(tok, "Delete me", "public")

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.do("DELETE", "/api/v1/anchors/"+target, tok, nil) }()
	}
	wg.Wait()

	var u struct {
		AnchorCount int `bson:"anchorCount"`
	}
	require.NoError(t, e.db.Collection("users").FindOne(context.Background(), bson.M{"_id": me}).Decode(&u))
	require.Equal(t, 2, u.AnchorCount)
}

func TestConcurrentPinsRespectLimit(t *testing.T) {
	e := newEnv(t)
	tok, me := e.login("pin@example.com")
	var ids []string
	for i := 0; i < 6; i++ {
		ids = append(ids, e.create(tok, fmt.Sprintf("Anchor %d", i), "public"))
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) { defer wg.Done(); e.do("PATCH", "/api/v1/anchors/"+id+"/pin", tok, nil) }(id)
	}
	wg.Wait()
	n, err := e.db.Collection("anchors").CountDocuments(context.Background(), bson.M{"userId": me, "isPinned": true})
	require.NoError(t, err)
	require.LessOrEqual(t, n, int64(maxPinnedAnchors))
}

func TestReorderRejectsForeignItemWithoutPartialWrite(t *testing.T) {
	e := newEnv(t)
	tok, _ := e.login("re@example.com")
	a := e.create(tok, "Anchor A", "public")
	b := e.create(tok, "Anchor B", "public")
	a1 := e.addText(tok, a, "first")
	a2 := e.addText(tok, a, "second")
	b1 := e.addText(tok, b, "other")

	code, out := e.do("PATCH", "/api/v1/anchors/"+a+"/items/reorder", tok, map[string][]string{"itemIds": {a2, b1}})
	require.Equal(t, 400, code)
	require.Contains(t, out["message"], "isn't in this anchor")

	code, out = e.do("GET", "/api/v1/anchors/"+a, tok, nil)
	require.Equal(t, 200, code)
	items := data(out)["items"].([]interface{})
	require.Equal(t, a1, items[0].(map[string]interface{})["id"], "order must be unchanged")

	// An item of another anchor can't be deleted through this anchor
	code, _ = e.do("DELETE", "/api/v1/anchors/"+a+"/items/"+b1, tok, nil)
	require.Equal(t, 404, code)
}

func TestNonOwnerEditing(t *testing.T) {
	e := newEnv(t)
	ownerTok, _ := e.login("own@example.com")
	otherTok, _ := e.login("oth@example.com")
	private := e.create(ownerTok, "Secret anchor", "private")
	public := e.create(ownerTok, "Public anchor", "public")

	code, _ := e.do("PATCH", "/api/v1/anchors/"+private, otherTok, map[string]string{"title": "Hacked"})
	require.Equal(t, 404, code, "someone else's private anchor must look non-existent")
	code, out := e.do("PATCH", "/api/v1/anchors/"+public, otherTok, map[string]string{"title": "Hacked"})
	require.Equal(t, 403, code)
	require.Equal(t, "Only the owner of this anchor can change it", out["message"])
}
