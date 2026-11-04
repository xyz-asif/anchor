package push

import (
	"context"
	"log"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Interfaces the dispatcher needs, so it can be tested without a database

type deviceStore interface {
	DevicesForUsers(ctx context.Context, userIDs []primitive.ObjectID) (map[primitive.ObjectID][]Device, error)
	SettingsForUsers(ctx context.Context, userIDs []primitive.ObjectID) (map[primitive.ObjectID]Settings, error)
	DeleteTokens(ctx context.Context, tokens []string) error
}

type userLookup interface {
	GetUsersByIDs(ctx context.Context, userIDs []primitive.ObjectID) ([]auth.User, error)
}

type unreadCounter interface {
	CountUnread(ctx context.Context, userID primitive.ObjectID) (int64, error)
}

const (
	queueSize       = 1000 // batches waiting to be sent; beyond this, pushes are dropped
	workerCount     = 4    // batches processed in parallel
	chunkSize       = 100  // a big batch (an anchor update to 5,000 followers) is sent in chunks...
	sendConcurrency = 8    // FCM requests in flight per chunk
)

// processTimeout is each chunk's own time limit (DB lookups + FCM calls); a variable for tests
var processTimeout = 20 * time.Second

// Dispatcher turns saved notifications into push messages in the background.
// It implements notifications.PushDispatcher.
type Dispatcher struct {
	store  deviceStore
	users  userLookup
	unread unreadCounter
	sender Sender
	queue  chan []notifications.Notification
	wg     sync.WaitGroup
	once   sync.Once
}

func NewDispatcher(store deviceStore, users userLookup, unread unreadCounter, sender Sender) *Dispatcher {
	d := &Dispatcher{
		store: store, users: users, unread: unread, sender: sender,
		queue: make(chan []notifications.Notification, queueSize),
	}
	for i := 0; i < workerCount; i++ {
		d.wg.Add(1)
		go d.worker()
	}
	return d
}

// Dispatch queues notifications for sending; it never blocks the request that created them
func (d *Dispatcher) Dispatch(batch []notifications.Notification) {
	if len(batch) == 0 {
		return
	}
	defer func() { recover() }() // queue closed during shutdown
	select {
	case d.queue <- batch:
	default:
		log.Printf("push: queue full, dropped %d notification(s)", len(batch))
	}
}

// Close stops accepting pushes and waits for queued ones to be sent
func (d *Dispatcher) Close() {
	d.once.Do(func() { close(d.queue) })
	d.wg.Wait()
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for batch := range d.queue {
		// One time limit for a whole 5,000-follower batch would expire after the first few
		// hundred sends and silently drop the rest, so each chunk gets its own
		for start := 0; start < len(batch); start += chunkSize {
			d.processChunk(batch[start:min(start+chunkSize, len(batch))])
		}
	}
}

func (d *Dispatcher) processChunk(chunk []notifications.Notification) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("push: panic while sending: %v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), processTimeout)
	defer cancel()
	d.process(ctx, chunk)
}

// process sends one push per notification to all of the recipient's devices
func (d *Dispatcher) process(ctx context.Context, batch []notifications.Notification) {
	recipientIDs := uniqueIDs(batch, func(n notifications.Notification) primitive.ObjectID { return n.RecipientID })

	settings, err := d.store.SettingsForUsers(ctx, recipientIDs)
	if err != nil {
		log.Printf("push: loading settings: %v", err)
		return
	}
	var wanted []notifications.Notification
	for _, n := range batch {
		if settings[n.RecipientID].Allows(n.Type) {
			wanted = append(wanted, n)
		}
	}
	if len(wanted) == 0 {
		return
	}

	devices, err := d.store.DevicesForUsers(ctx, recipientIDs)
	if err != nil {
		log.Printf("push: loading devices: %v", err)
		return
	}
	actorNames := d.actorNames(ctx, uniqueIDs(wanted, func(n notifications.Notification) primitive.ObjectID { return n.ActorID }))

	// Sends run in parallel (each is a network round trip to FCM), a few at a time
	var (
		mu   sync.Mutex
		dead []string
		wg   sync.WaitGroup
		sem  = make(chan struct{}, sendConcurrency)
	)
	for _, n := range wanted {
		userDevices := devices[n.RecipientID]
		if len(userDevices) == 0 {
			continue
		}
		tokens := make([]string, len(userDevices))
		for i, dev := range userDevices {
			tokens[i] = dev.Token
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(n notifications.Notification, tokens []string) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("push: panic while sending: %v", r)
				}
			}()

			title, body := MessageText(n, actorNames[n.ActorID])
			msg := Message{Tokens: tokens, Title: title, Body: body, Data: messageData(n)}
			if count, err := d.unread.CountUnread(ctx, n.RecipientID); err == nil {
				badge := int(count)
				msg.Badge = &badge
			}

			invalid, err := d.sender.Send(ctx, msg)
			if err != nil {
				log.Printf("push: sending %s notification %s: %v", n.Type, n.ID.Hex(), err)
			}
			if len(invalid) > 0 {
				mu.Lock()
				dead = append(dead, invalid...)
				mu.Unlock()
			}
		}(n, tokens)
	}
	wg.Wait()

	if len(dead) > 0 {
		if err := d.store.DeleteTokens(ctx, dead); err != nil {
			log.Printf("push: removing %d invalid token(s): %v", len(dead), err)
		}
	}
}

// actorNames maps user IDs to the name shown in pushes
func (d *Dispatcher) actorNames(ctx context.Context, ids []primitive.ObjectID) map[primitive.ObjectID]string {
	names := make(map[primitive.ObjectID]string, len(ids))
	users, err := d.users.GetUsersByIDs(ctx, ids)
	if err != nil {
		return names
	}
	for _, u := range users {
		switch {
		case u.DisplayName != "":
			names[u.ID] = u.DisplayName
		case u.Username != "":
			names[u.ID] = "@" + u.Username
		}
	}
	return names
}

// MessageText builds the push title and body for a notification
func MessageText(n notifications.Notification, actor string) (title, body string) {
	if actor == "" {
		actor = "Someone"
	}
	preview := shorten(n.Preview, 80)
	quoted := func(s string) string {
		if s == "" {
			return ""
		}
		return " \"" + s + "\""
	}
	withText := func(action string) string {
		if preview == "" {
			return actor + " " + action
		}
		return actor + " " + action + ": " + preview
	}

	switch n.Type {
	case notifications.TypeLike:
		return "New like", actor + " liked your anchor" + quoted(preview)
	case notifications.TypeComment:
		return "New comment", withText("commented")
	case notifications.TypeReply:
		return "New reply", withText("replied to your comment")
	case notifications.TypeMention:
		return "You were mentioned", withText("mentioned you")
	case notifications.TypeFollow:
		return "New follower", actor + " started following you"
	case notifications.TypeClone:
		return "Anchor cloned", actor + " cloned your anchor" + quoted(preview)
	case notifications.TypeAnchorUpdate:
		return "Anchor updated", actor + " added to" + quoted(preview)
	}
	return "New notification", withText("interacted with you")
}

// messageData is the payload the app uses to open the right screen
func messageData(n notifications.Notification) map[string]string {
	data := map[string]string{
		"type":           n.Type,
		"notificationId": n.ID.Hex(),
		"actorId":        n.ActorID.Hex(),
		"resourceType":   n.ResourceType,
		"resourceId":     n.ResourceID.Hex(),
	}
	if n.AnchorID != nil {
		data["anchorId"] = n.AnchorID.Hex()
	}
	return data
}

// shorten cuts s to at most max characters (not bytes), adding "…" when cut
func shorten(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

func uniqueIDs(batch []notifications.Notification, key func(notifications.Notification) primitive.ObjectID) []primitive.ObjectID {
	seen := make(map[primitive.ObjectID]bool, len(batch))
	var out []primitive.ObjectID
	for _, n := range batch {
		id := key(n)
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
