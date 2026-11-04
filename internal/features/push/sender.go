package push

import (
	"context"
	"fmt"
	"log"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// Message is one push notification sent to all devices of one user
type Message struct {
	Tokens []string
	Title  string
	Body   string
	Data   map[string]string // deep-link info for the app (all values are strings in FCM)
	Badge  *int              // unread count shown on the iOS app icon
}

// Sender delivers push messages. It returns the tokens that are no longer valid
// (app uninstalled, token rotated), which the caller removes.
type Sender interface {
	Send(ctx context.Context, msg Message) (invalidTokens []string, err error)
}

// fcmMaxTokens is the most tokens FCM accepts in one multicast request
const fcmMaxTokens = 500

// FCMSender sends pushes through Firebase Cloud Messaging
type FCMSender struct {
	client *messaging.Client
	dryRun bool // validate with FCM without delivering (for testing credentials)
}

// NewFCMSender creates an FCM sender from a Firebase service-account file
func NewFCMSender(ctx context.Context, credentialsPath string, dryRun bool) (*FCMSender, error) {
	app, err := firebase.NewApp(ctx, nil, option.WithAuthCredentialsFile(option.ServiceAccount, credentialsPath))
	if err != nil {
		return nil, fmt.Errorf("initialising firebase: %w", err)
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialising firebase messaging: %w", err)
	}
	return &FCMSender{client: client, dryRun: dryRun}, nil
}

func (s *FCMSender) Send(ctx context.Context, msg Message) ([]string, error) {
	var invalid []string
	var firstErr error
	for start := 0; start < len(msg.Tokens); start += fcmMaxTokens {
		end := min(start+fcmMaxTokens, len(msg.Tokens))
		tokens := msg.Tokens[start:end]

		multicast := buildFCMMessage(msg, tokens)
		var (
			resp *messaging.BatchResponse
			err  error
		)
		if s.dryRun {
			resp, err = s.client.SendEachForMulticastDryRun(ctx, multicast)
		} else {
			resp, err = s.client.SendEachForMulticast(ctx, multicast)
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for i, r := range resp.Responses {
			if r.Success || r.Error == nil {
				continue
			}
			if isDeadToken(r.Error) {
				invalid = append(invalid, tokens[i])
			} else if firstErr == nil {
				firstErr = r.Error
			}
		}
	}
	return invalid, firstErr
}

// isDeadToken reports whether FCM rejected the token itself (so it should be deleted),
// as opposed to a temporary or server-side problem.
func isDeadToken(err error) bool {
	return messaging.IsUnregistered(err) || messaging.IsRegistrationTokenNotRegistered(err) ||
		messaging.IsSenderIDMismatch(err)
}

func buildFCMMessage(msg Message, tokens []string) *messaging.MulticastMessage {
	collapse := msg.Data["type"] // newer pushes of the same kind replace older ones on Android
	return &messaging.MulticastMessage{
		Tokens:       tokens,
		Data:         msg.Data,
		Notification: &messaging.Notification{Title: msg.Title, Body: msg.Body},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				Sound: "default",
				Tag:   collapse + ":" + msg.Data["resourceId"],
			},
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{Badge: msg.Badge, Sound: "default", ThreadID: collapse},
			},
		},
	}
}

// LogSender only logs pushes (for local development without real devices)
type LogSender struct{}

func (LogSender) Send(_ context.Context, msg Message) ([]string, error) {
	badge := "-"
	if msg.Badge != nil {
		badge = fmt.Sprint(*msg.Badge)
	}
	log.Printf("PUSH (log mode) to %d device(s): %q / %q badge=%s data=%v",
		len(msg.Tokens), msg.Title, msg.Body, badge, msg.Data)
	return nil, nil
}
