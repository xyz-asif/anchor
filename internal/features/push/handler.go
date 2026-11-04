package push

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/bind"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func currentUser(c *gin.Context) (*auth.User, bool) {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return user, true
		}
	}
	response.Unauthorized(c, "Please sign in first", "UNAUTHORIZED")
	return nil, false
}

const (
	minTokenLength = 20
	maxTokenLength = 4096
	maxAppVersion  = 50
)

// ValidateDevice cleans a device registration and explains any problem
func ValidateDevice(req *RegisterDeviceRequest) error {
	req.Token = strings.TrimSpace(req.Token)
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.AppVersion = strings.TrimSpace(req.AppVersion)
	if err := validateToken(req.Token); err != nil {
		return err
	}
	switch req.Platform {
	case PlatformAndroid, PlatformIOS, PlatformWeb:
	case "":
		return errors.New(`Please send the platform: "android", "ios" or "web"`)
	default:
		return fmt.Errorf(`"%s" isn't a supported platform. Use "android", "ios" or "web"`, req.Platform)
	}
	if utf8.RuneCountInString(req.AppVersion) > maxAppVersion {
		return fmt.Errorf("appVersion can be at most %d characters long", maxAppVersion)
	}
	return nil
}

func validateToken(token string) error {
	switch {
	case token == "":
		return errors.New(`Please send this device's push token as "token"`)
	case len(token) < minTokenLength || len(token) > maxTokenLength:
		return fmt.Errorf("This doesn't look like a push token (it should be %d to %d characters long)", minTokenLength, maxTokenLength)
	case strings.ContainsAny(token, " \t\r\n/"):
		return errors.New("This push token contains spaces or slashes, which real tokens never do")
	}
	return nil
}

// RegisterDevice godoc
// @Summary Register a device for push notifications
// @Description Saves the FCM token of this app install. Call it after login and whenever the
// @Description app gets a new token. Re-registering the same token is safe; a token used by
// @Description another account before moves to the current user. Pushes stop when this login
// @Description session ends (logout, "log out everywhere", or expiry).
// @Tags push
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body RegisterDeviceRequest true "FCM token and platform"
// @Success 200 {object} response.APIResponse{data=Device}
// @Failure 400 {object} response.APIResponse
// @Router /devices [post]
func (h *Handler) RegisterDevice(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	var req RegisterDeviceRequest
	if !bind.JSON(c, &req) {
		return
	}
	if err := ValidateDevice(&req); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_DEVICE")
		return
	}

	device, err := h.repo.UpsertDevice(c.Request.Context(), user.ID, c.GetString(auth.SessionIDKey), req)
	if err != nil {
		log.Printf("push: registering device for %s failed: %v", user.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't turn on push notifications for this device. Please try again", "DEVICE_REGISTER_FAILED")
		return
	}
	response.Success(c, device, "Push notifications are on for this device")
}

// UnregisterDevice godoc
// @Summary Stop push notifications on a device
// @Description Call before logging out so this phone stops receiving the user's pushes. Send the
// @Description token in the body (DELETE /devices) or, for older apps, in the path. Unregistering
// @Description a token that isn't registered is not an error, so it's safe to call on every logout.
// @Tags push
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UnregisterDeviceRequest false "FCM token"
// @Success 200 {object} response.APIResponse
// @Router /devices [delete]
func (h *Handler) UnregisterDevice(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	token := c.Param("token")
	if token == "" {
		var req UnregisterDeviceRequest
		if !bind.JSON(c, &req) {
			return
		}
		token = req.Token
	}
	token = strings.TrimSpace(token)
	if err := validateToken(token); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_DEVICE")
		return
	}

	removed, err := h.repo.DeleteDevice(c.Request.Context(), user.ID, token)
	if err != nil {
		log.Printf("push: unregistering device for %s failed: %v", user.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't turn off push notifications for this device. Please try again", "DEVICE_UNREGISTER_FAILED")
		return
	}
	response.Success(c, gin.H{"removed": removed}, "Push notifications are off for this device")
}

// GetSettings godoc
// @Summary Get push notification settings
// @Description Which kinds of notifications are sent as pushes. Everything is on by default.
// @Description In-app notifications are always created; these settings only affect pushes.
// @Tags push
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=Settings}
// @Router /users/me/notification-settings [get]
func (h *Handler) GetSettings(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	settings, err := h.repo.GetSettings(c.Request.Context(), user.ID)
	if err != nil {
		log.Printf("push: loading settings for %s failed: %v", user.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't load your notification settings. Please try again", "SETTINGS_FETCH_FAILED")
		return
	}
	response.Success(c, settings, "Notification settings retrieved")
}

// UpdateSettings godoc
// @Summary Update push notification settings
// @Description Send only the switches you want to change, e.g. {"likes": false}.
// @Description "enabled": false turns off all pushes.
// @Tags push
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateSettingsRequest true "Settings to change"
// @Success 200 {object} response.APIResponse{data=Settings}
// @Failure 400 {object} response.APIResponse
// @Router /users/me/notification-settings [patch]
func (h *Handler) UpdateSettings(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	var req UpdateSettingsRequest
	if !bind.JSON(c, &req) {
		return
	}
	if req.IsEmpty() {
		response.BadRequest(c, "Send at least one setting to change: enabled, likes, comments, replies, mentions, follows, clones, anchorUpdates", "NO_SETTINGS")
		return
	}

	settings, err := h.repo.UpdateSettings(c.Request.Context(), user.ID, req)
	if err != nil {
		log.Printf("push: saving settings for %s failed: %v", user.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't save your notification settings. Please try again", "SETTINGS_UPDATE_FAILED")
		return
	}
	response.Success(c, settings, "Notification settings updated")
}
