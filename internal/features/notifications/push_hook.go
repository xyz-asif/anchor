package notifications

import (
	"sync"
)

// PushDispatcher delivers saved notifications as push notifications to the recipients'
// devices. It is implemented by the push package and registered once at startup, so this
// package doesn't depend on push (which itself imports notifications).
type PushDispatcher interface {
	// Dispatch is called right after notifications are saved. It must not block the caller.
	Dispatch(notifications []Notification)
}

var (
	pushMu         sync.RWMutex
	pushDispatcher PushDispatcher
)

// SetPushDispatcher registers the dispatcher used for every saved notification (nil disables push)
func SetPushDispatcher(d PushDispatcher) {
	pushMu.Lock()
	defer pushMu.Unlock()
	pushDispatcher = d
}

// dispatchPush hands freshly saved notifications to the push dispatcher, if one is registered
func dispatchPush(notifications []Notification) {
	pushMu.RLock()
	d := pushDispatcher
	pushMu.RUnlock()
	if d == nil || len(notifications) == 0 {
		return
	}
	copied := make([]Notification, len(notifications))
	copy(copied, notifications)
	d.Dispatch(copied)
}
