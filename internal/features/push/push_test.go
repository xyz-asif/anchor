package push

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type fakeStore struct {
	mu       sync.Mutex
	devices  map[primitive.ObjectID][]Device
	settings map[primitive.ObjectID]Settings
	deleted  []string
}

func (f *fakeStore) DevicesForUsers(_ context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID][]Device, error) {
	out := map[primitive.ObjectID][]Device{}
	for _, id := range ids {
		out[id] = f.devices[id]
	}
	return out, nil
}

func (f *fakeStore) SettingsForUsers(_ context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]Settings, error) {
	out := map[primitive.ObjectID]Settings{}
	for _, id := range ids {
		if s, ok := f.settings[id]; ok {
			out[id] = s
		} else {
			out[id] = DefaultSettings(id)
		}
	}
	return out, nil
}

func (f *fakeStore) DeleteTokens(_ context.Context, tokens []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, tokens...)
	return nil
}

type fakeUsers struct{ users []auth.User }

func (f fakeUsers) GetUsersByIDs(context.Context, []primitive.ObjectID) ([]auth.User, error) {
	return f.users, nil
}

type fakeUnread struct{ n int64 }

func (f fakeUnread) CountUnread(context.Context, primitive.ObjectID) (int64, error) { return f.n, nil }

type fakeSender struct {
	mu      sync.Mutex
	sent    []Message
	invalid map[string]bool
	err     error
}

func (f *fakeSender) Send(_ context.Context, msg Message) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	var bad []string
	for _, t := range msg.Tokens {
		if f.invalid[t] {
			bad = append(bad, t)
		}
	}
	return bad, f.err
}

func TestDispatcher_SendsToAllDevicesWithTextBadgeAndData(t *testing.T) {
	alice, bob := primitive.NewObjectID(), primitive.NewObjectID()
	anchorID := primitive.NewObjectID()
	store := &fakeStore{devices: map[primitive.ObjectID][]Device{
		alice: {{UserID: alice, Token: "phone"}, {UserID: alice, Token: "tablet"}},
	}}
	sender := &fakeSender{}
	d := NewDispatcher(store, fakeUsers{[]auth.User{{ID: bob, DisplayName: "Bob"}}}, fakeUnread{3}, sender)

	n := notifications.Notification{
		ID: primitive.NewObjectID(), RecipientID: alice, ActorID: bob, Type: notifications.TypeLike,
		ResourceType: "anchor", ResourceID: anchorID, AnchorID: &anchorID, Preview: "Go tips",
	}
	d.process(context.Background(), []notifications.Notification{n})

	require.Len(t, sender.sent, 1)
	msg := sender.sent[0]
	require.ElementsMatch(t, []string{"phone", "tablet"}, msg.Tokens)
	require.Equal(t, "New like", msg.Title)
	require.Equal(t, `Bob liked your anchor "Go tips"`, msg.Body)
	require.NotNil(t, msg.Badge)
	require.Equal(t, 3, *msg.Badge)
	require.Equal(t, map[string]string{
		"type": "like", "notificationId": n.ID.Hex(), "actorId": bob.Hex(),
		"resourceType": "anchor", "resourceId": anchorID.Hex(), "anchorId": anchorID.Hex(),
	}, msg.Data)
}

func TestDispatcher_RespectsSettings(t *testing.T) {
	alice, carol, bob := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	likesOff := DefaultSettings(alice)
	likesOff.Likes = false
	allOff := DefaultSettings(carol)
	allOff.Enabled = false
	store := &fakeStore{
		devices: map[primitive.ObjectID][]Device{
			alice: {{UserID: alice, Token: "a"}},
			carol: {{UserID: carol, Token: "c"}},
		},
		settings: map[primitive.ObjectID]Settings{alice: likesOff, carol: allOff},
	}
	sender := &fakeSender{}
	d := NewDispatcher(store, fakeUsers{}, fakeUnread{}, sender)

	d.process(context.Background(), []notifications.Notification{
		{RecipientID: alice, ActorID: bob, Type: notifications.TypeLike},    // off for alice
		{RecipientID: alice, ActorID: bob, Type: notifications.TypeFollow},  // still on
		{RecipientID: carol, ActorID: bob, Type: notifications.TypeComment}, // carol turned off all pushes
	})

	require.Len(t, sender.sent, 1)
	require.Equal(t, "New follower", sender.sent[0].Title)
	require.Equal(t, "Someone started following you", sender.sent[0].Body) // unknown actor
}

func TestDispatcher_RemovesDeadTokensAndSkipsUsersWithoutDevices(t *testing.T) {
	alice, dave, bob := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	store := &fakeStore{devices: map[primitive.ObjectID][]Device{
		alice: {{UserID: alice, Token: "live"}, {UserID: alice, Token: "uninstalled"}},
	}}
	sender := &fakeSender{invalid: map[string]bool{"uninstalled": true}, err: errors.New("partial failure")}
	d := NewDispatcher(store, fakeUsers{}, fakeUnread{}, sender)

	d.process(context.Background(), []notifications.Notification{
		{RecipientID: alice, ActorID: bob, Type: notifications.TypeComment, Preview: "nice"},
		{RecipientID: dave, ActorID: bob, Type: notifications.TypeComment},
	})

	require.Len(t, sender.sent, 1, "dave has no devices")
	require.Equal(t, []string{"uninstalled"}, store.deleted)
}

func TestDispatcher_DispatchIsAsyncAndCloseDrains(t *testing.T) {
	alice := primitive.NewObjectID()
	store := &fakeStore{devices: map[primitive.ObjectID][]Device{alice: {{UserID: alice, Token: "t"}}}}
	sender := &fakeSender{}
	d := NewDispatcher(store, fakeUsers{}, fakeUnread{}, sender)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			d.Dispatch([]notifications.Notification{{RecipientID: alice, Type: notifications.TypeFollow}})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Dispatch blocked")
	}
	d.Close()
	require.Len(t, sender.sent, 5)
	d.Dispatch([]notifications.Notification{{RecipientID: alice}}) // after Close: ignored, no panic
}

func TestMessageText(t *testing.T) {
	cases := []struct {
		typ, preview, title, body string
	}{
		{notifications.TypeComment, "Great list!", "New comment", "Ann commented: Great list!"},
		{notifications.TypeReply, "Thanks", "New reply", "Ann replied to your comment: Thanks"},
		{notifications.TypeMention, "hey @you", "You were mentioned", "Ann mentioned you: hey @you"},
		{notifications.TypeClone, "Recipes", "Anchor cloned", `Ann cloned your anchor "Recipes"`},
		{notifications.TypeAnchorUpdate, "Recipes", "Anchor updated", `Ann added to "Recipes"`},
		{notifications.TypeComment, "", "New comment", "Ann commented"},
	}
	for _, c := range cases {
		title, body := MessageText(notifications.Notification{Type: c.typ, Preview: c.preview}, "Ann")
		require.Equal(t, c.title, title, c.typ)
		require.Equal(t, c.body, body, c.typ)
	}

	long := ""
	for i := 0; i < 100; i++ {
		long += "é"
	}
	_, body := MessageText(notifications.Notification{Type: notifications.TypeComment, Preview: long}, "Ann")
	require.Equal(t, "Ann commented: "+string([]rune(long)[:79])+"…", body)
}

func TestSettings(t *testing.T) {
	s := DefaultSettings(primitive.NewObjectID())
	for _, typ := range []string{"like", "comment", "reply", "mention", "follow", "clone", "anchor_update", "future_type"} {
		require.True(t, s.Allows(typ), typ)
	}
	off := false
	UpdateSettingsRequest{Replies: &off}.Apply(&s)
	require.False(t, s.Allows("reply"))
	require.True(t, s.Allows("comment"))
	require.True(t, UpdateSettingsRequest{}.IsEmpty())
	require.False(t, UpdateSettingsRequest{Replies: &off}.IsEmpty())
}

func TestBuildFCMMessage(t *testing.T) {
	badge := 2
	m := buildFCMMessage(Message{
		Title: "New like", Body: "Bob liked", Badge: &badge,
		Data: map[string]string{"type": "like", "resourceId": "abc"},
	}, []string{"t1"})
	require.Equal(t, []string{"t1"}, m.Tokens)
	require.Equal(t, "high", m.Android.Priority)
	require.Equal(t, "like:abc", m.Android.Notification.Tag)
	require.Equal(t, 2, *m.APNS.Payload.Aps.Badge)
	require.Equal(t, "New like", m.Notification.Title)
}
