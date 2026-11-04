package push

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	activeMu         sync.Mutex
	activeDispatcher *Dispatcher
)

// RegisterRoutes adds the device and settings endpoints and starts sending pushes for
// new notifications, using the sender chosen by PUSH_MODE.
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)
	handler := NewHandler(repo)
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)

	devices := router.Group("/devices", authMiddleware)
	{
		devices.POST("", handler.RegisterDevice)
		devices.DELETE("", handler.UnregisterDevice)        // token in the body
		devices.DELETE("/:token", handler.UnregisterDevice) // older apps: token in the path
	}
	settings := router.Group("/users/me/notification-settings", authMiddleware)
	{
		settings.GET("", handler.GetSettings)
		settings.PATCH("", handler.UpdateSettings)
	}

	sender := newSender(cfg)
	if sender == nil {
		notifications.SetPushDispatcher(nil)
		return
	}
	d := NewDispatcher(repo, authRepo, notifications.NewRepository(db), sender)
	notifications.SetPushDispatcher(d)

	activeMu.Lock()
	old := activeDispatcher
	activeDispatcher = d
	activeMu.Unlock()
	if old != nil {
		old.Close()
	}
}

// Shutdown stops sending pushes, after the queued ones are sent (call on server shutdown)
func Shutdown() {
	notifications.SetPushDispatcher(nil)
	activeMu.Lock()
	d := activeDispatcher
	activeDispatcher = nil
	activeMu.Unlock()
	if d != nil {
		d.Close()
	}
}

// newSender picks the sender for PUSH_MODE:
//   - fcm:  send through Firebase (fails loudly if the credentials are missing)
//   - log:  only log pushes (local development)
//   - off:  no pushes
//   - auto (default): fcm if the Firebase service-account file exists, otherwise off
func newSender(cfg *config.Config) Sender {
	mode := strings.ToLower(strings.TrimSpace(cfg.PushMode))
	switch mode {
	case "off", "none", "disabled":
		log.Println("Push notifications: off (PUSH_MODE=off)")
		return nil
	case "log":
		log.Println("Push notifications: log mode, pushes are logged instead of sent")
		return LogSender{}
	case "fcm", "fcm-dry-run":
		sender, err := NewFCMSender(context.Background(), cfg.FirebaseServiceAccountPath, mode == "fcm-dry-run")
		if err != nil {
			log.Printf("PUSH WARNING: PUSH_MODE=%s but Firebase could not be initialised (%v); pushes are off", mode, err)
			return nil
		}
		log.Printf("Push notifications: Firebase Cloud Messaging (%s)", mode)
		return sender
	}

	if mode != "" && mode != "auto" {
		log.Printf("PUSH WARNING: unknown PUSH_MODE %q (use fcm, log, off or auto); using auto", cfg.PushMode)
	}
	if _, err := os.Stat(cfg.FirebaseServiceAccountPath); err != nil {
		log.Println("Push notifications: off (no Firebase service-account file; set FIREBASE_SERVICE_ACCOUNT_PATH or PUSH_MODE=log)")
		return nil
	}
	sender, err := NewFCMSender(context.Background(), cfg.FirebaseServiceAccountPath, false)
	if err != nil {
		log.Printf("PUSH WARNING: Firebase could not be initialised (%v); pushes are off", err)
		return nil
	}
	log.Println("Push notifications: Firebase Cloud Messaging")
	return sender
}
