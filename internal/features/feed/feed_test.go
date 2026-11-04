package feed

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"github.com/xyz-asif/gotodo/internal/features/anchor_follows"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------- unit tests ----------

func TestCursorsSurviveQueryStrings(t *testing.T) {
	id := primitive.NewObjectID()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c := EncodeCursor(ts, id)
	require.NotContains(t, c, "+")
	require.NotContains(t, c, "/")
	require.NotContains(t, c, "=")

	got, err := DecodeCursor(c)
	require.NoError(t, err)
	require.Equal(t, id, got.AnchorID)
	require.True(t, ts.Equal(got.Timestamp))

	// An older standard-base64 cursor whose '+' became a space in an unencoded query string
	raw, _ := json.Marshal(FeedCursor{Timestamp: ts, AnchorID: id})
	std := base64.StdEncoding.EncodeToString(raw)
	got, err = DecodeCursor(strings.ReplaceAll(std, "+", " "))
	require.NoError(t, err)
	require.Equal(t, id, got.AnchorID)

	_, err = DecodeCursor("not-a-cursor!!")
	require.ErrorIs(t, err, errBadCursor)
	_, err = DecodeCursor(strings.Repeat("a", 2000))
	require.ErrorIs(t, err, errBadCursor)
}

func TestParseQueries(t *testing.T) {
	q, err := ParseFeedQuery("500", "", "")
	require.NoError(t, err)
	require.Equal(t, 50, q.Limit, "a large limit is capped, not rejected")
	require.True(t, q.IncludeOwn)

	_, err = ParseFeedQuery("abc", "", "")
	require.EqualError(t, err, `Limit must be a whole number between 1 and 50 (got "abc")`)
	_, err = ParseFeedQuery("", "", "yes")
	require.EqualError(t, err, `includeOwn must be true or false (got "yes")`)

	_, err = ParseDiscoverQuery("", "", "hot", "", CategoryTrending)
	require.EqualError(t, err, `"hot" isn't a feed category. Use trending, popular, recent or for_you`)
	_, err = ParseDiscoverQuery("", "", "", "#", CategoryTrending)
	require.ErrorContains(t, err, `"#" isn't a valid tag`)

	d, err := ParseDiscoverQuery("", "", "For-You", "#Web Design", CategoryTrending)
	require.NoError(t, err)
	require.Equal(t, CategoryForYou, d.Category)
	require.Equal(t, "web-design", d.Tag)

	// A "recent" cursor (no score) can't page a score-sorted category
	recent := EncodeDiscoverCursor(nil, time.Now(), primitive.NewObjectID())
	_, err = ParseDiscoverQuery("", recent, "popular", "", CategoryTrending)
	require.ErrorIs(t, err, errBadCursor)
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
	db := client.Database(fmt.Sprintf("feed_test_%d", time.Now().UnixNano()))
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
	anchors.RegisterRoutes(api, db, cfg, anchor_follows.NewRepository(db), notifications.GetService(db))
	anchor_follows.RegisterRoutes(api, db, cfg)
	follows.RegisterRoutes(api, db, cfg)
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

func (e *env) create(token, title string, tagList ...string) string {
	body := map[string]interface{}{"title": title, "visibility": "public"}
	if len(tagList) > 0 {
		body["tags"] = tagList
	}
	code, out := e.do("POST", "/api/v1/anchors", token, body)
	require.Equal(e.t, 201, code, out)
	return out["data"].(map[string]interface{})["id"].(string)
}

func (e *env) follow(token string, user primitive.ObjectID) {
	code, out := e.do("POST", "/api/v1/users/"+user.Hex()+"/follow", token, map[string]string{"action": "follow"})
	require.Equal(e.t, 200, code, out)
}

func (e *env) setUser(id primitive.ObjectID, set bson.M) {
	_, err := e.db.Collection("users").UpdateOne(context.Background(), bson.M{"_id": id}, bson.M{"$set": set})
	require.NoError(e.t, err)
}

func titles(items interface{}) []string {
	var out []string
	for _, it := range items.([]interface{}) {
		out = append(out, it.(map[string]interface{})["title"].(string))
	}
	return out
}

func homeFeed(e *env, token, query string) map[string]interface{} {
	code, out := e.do("GET", "/api/v1/feed/following"+query, token, nil)
	require.Equal(e.t, 200, code, out)
	return out["data"].(map[string]interface{})
}

func TestUsersWhoBlockedYouDisappearFromFeeds(t *testing.T) {
	e := newEnv(t)
	meTok, me := e.login("me@example.com")
	blockerTok, blocker := e.login("blocker@example.com")
	friendTok, friend := e.login("friend@example.com")
	e.create(blockerTok, "From the blocker", "travel")
	e.create(friendTok, "From a friend", "travel")
	e.follow(meTok, blocker)
	e.follow(meTok, friend)

	// The blocker blocks me, but removing the follow failed (it is best effort), so I still follow them
	e.setUser(blocker, bson.M{"blockedUsers": []primitive.ObjectID{me}})

	feed := homeFeed(e, meTok, "")["feed"].(map[string]interface{})
	require.Equal(t, []string{"From a friend"}, titles(feed["items"]))

	code, out := e.do("GET", "/api/v1/feed/tags/travel", meTok, nil)
	require.Equal(t, 200, code, out)
	require.Equal(t, []string{"From a friend"}, titles(out["data"].(map[string]interface{})["items"]))
}

func TestLikedByFollowingFindsOlderLikes(t *testing.T) {
	e := newEnv(t)
	meTok, _ := e.login("me2@example.com")
	authorTok, author := e.login("author2@example.com")
	_, friend := e.login("friend2@example.com")
	e.follow(meTok, author)
	e.follow(meTok, friend)
	a := e.create(authorTok, "Much liked")
	anchorID, _ := primitive.ObjectIDFromHex(a)

	// My friend liked it first; 25 strangers liked it afterwards
	ctx := context.Background()
	start := time.Now().Add(-time.Hour)
	likesColl := e.db.Collection("likes")
	_, err := likesColl.InsertOne(ctx, bson.M{"anchorId": anchorID, "userId": friend, "createdAt": start})
	require.NoError(t, err)
	for i := 0; i < 25; i++ {
		_, err := likesColl.InsertOne(ctx, bson.M{"anchorId": anchorID, "userId": primitive.NewObjectID(), "createdAt": start.Add(time.Duration(i+1) * time.Minute)})
		require.NoError(t, err)
	}
	_, err = e.db.Collection("anchors").UpdateOne(ctx, bson.M{"_id": anchorID}, bson.M{"$set": bson.M{"likeCount": 26}})
	require.NoError(t, err)

	items := homeFeed(e, meTok, "")["feed"].(map[string]interface{})["items"].([]interface{})
	require.Len(t, items, 1)
	summary := items[0].(map[string]interface{})["engagement"].(map[string]interface{})["likeSummary"].(map[string]interface{})
	likedBy := summary["likedByFollowing"].([]interface{})
	require.Len(t, likedBy, 1, "a friend's like must be found even when 25 newer likes came after it")
	require.Equal(t, friend.Hex(), likedBy[0].(map[string]interface{})["id"])
	require.EqualValues(t, 25, summary["otherLikersCount"])
}

func TestFollowedAnchorsSectionSkipsDeletedAnchors(t *testing.T) {
	e := newEnv(t)
	meTok, _ := e.login("me3@example.com")
	ownerTok, _ := e.login("owner3@example.com")
	var ids []string
	for i := 0; i < 12; i++ {
		id := e.create(ownerTok, fmt.Sprintf("Anchor %02d", i))
		code, out := e.do("POST", "/api/v1/anchors/"+id+"/follow", meTok, map[string]interface{}{"action": "follow"})
		require.Equal(t, 200, code, out)
		ids = append(ids, id)
	}
	// The owner deletes the 3 anchors I followed last
	for _, id := range ids[9:] {
		code, _ := e.do("DELETE", "/api/v1/anchors/"+id, ownerTok, nil)
		require.Equal(t, 200, code)
	}

	section := homeFeed(e, meTok, "")["followingAnchors"].(map[string]interface{})
	require.EqualValues(t, 9, section["totalCount"], "deleted anchors aren't counted")
	require.Len(t, section["items"], 9, "the section is filled with anchors that still exist")
}

func TestForYouFallsBackWhenOnlyHiddenAnchorsMatch(t *testing.T) {
	e := newEnv(t)
	meTok, me := e.login("me4@example.com")
	followedTok, followed := e.login("followed4@example.com")
	otherTok, _ := e.login("other4@example.com")
	e.follow(meTok, followed)
	e.create(followedTok, "Travel by someone I follow", "travel")
	e.create(otherTok, "Cooking by a stranger", "cooking")
	e.setUser(me, bson.M{"interests": []string{"travel"}})

	// Discover leaves out people I follow, so nothing matches my interests: show popular instead
	code, out := e.do("GET", "/api/v1/feed/discover?category=for_you", meTok, nil)
	require.Equal(t, 200, code, out)
	require.Equal(t, []string{"Cooking by a stranger"}, titles(out["data"].(map[string]interface{})["items"]))
}

func TestFeedPagingAndMessages(t *testing.T) {
	e := newEnv(t)
	meTok, _ := e.login("me5@example.com")
	for i := 0; i < 3; i++ {
		e.create(meTok, fmt.Sprintf("Mine %d", i), "books")
	}

	page1 := homeFeed(e, meTok, "?limit=2")["feed"].(map[string]interface{})
	require.Len(t, page1["items"], 2)
	cursor := page1["pagination"].(map[string]interface{})["nextCursor"].(string)
	page2 := homeFeed(e, meTok, "?limit=2&cursor="+url.QueryEscape(cursor))["feed"].(map[string]interface{})
	require.Len(t, page2["items"], 1)
	require.Nil(t, page2["pagination"].(map[string]interface{})["nextCursor"])

	// The tag feed's last page says the feed ended, not that the tag has no content
	code, out := e.do("GET", "/api/v1/feed/tags/books?category=recent&limit=3", meTok, nil)
	require.Equal(t, 200, code, out)
	require.Len(t, out["data"].(map[string]interface{})["items"], 3)
	last := EncodeDiscoverCursor(nil, time.Unix(1, 0), primitive.NewObjectID())
	code, out = e.do("GET", "/api/v1/feed/tags/books?category=recent&cursor="+last, meTok, nil)
	require.Equal(t, 200, code, out)
	require.Equal(t, "END_OF_FEED", out["data"].(map[string]interface{})["meta"].(map[string]interface{})["emptyReason"])

	code, out = e.do("GET", "/api/v1/feed/following?cursor=garbage", meTok, nil)
	require.Equal(t, 400, code)
	require.Equal(t, "This feed position is invalid or out of date. Please refresh the feed and try again", out["message"])

	code, out = e.do("GET", "/api/v1/feed/discover?limit=abc", "", nil)
	require.Equal(t, 400, code)
	require.Equal(t, `Limit must be a whole number between 1 and 50 (got "abc")`, out["message"])

	code, out = e.do("GET", "/api/v1/feed/following", "", nil)
	require.Equal(t, 401, code)
}
