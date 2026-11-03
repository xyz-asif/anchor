package notifications

import (
	"context"
	"time"

	"github.com/xyz-asif/gotodo/internal/features/auth"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// FollowerProvider defines an interface to fetch anchor followers
// This helps break the circular dependency with anchor_follows package
type FollowerProvider interface {
	GetFollowersWithNotifications(ctx context.Context, anchorID primitive.ObjectID) ([]primitive.ObjectID, error)
}

type Service struct {
	repo             *Repository
	authRepo         *auth.Repository
	db               *mongo.Database
	followerProvider FollowerProvider
}

func NewService(repo *Repository, authRepo *auth.Repository, db *mongo.Database) *Service {
	return &Service{
		repo:     repo,
		authRepo: authRepo,
		db:       db,
	}
}

func (s *Service) SetFollowerProvider(provider FollowerProvider) {
	s.followerProvider = provider
}

// CommentData holds data needed for notification creation
type CommentData struct {
	ID       primitive.ObjectID
	AnchorID primitive.ObjectID
	Content  string
	Mentions []primitive.ObjectID
}

// CreateCommentNotifications creates notifications for a new comment
func (s *Service) CreateCommentNotifications(ctx context.Context, comment *CommentData, anchorID primitive.ObjectID, anchorUserID primitive.ObjectID, actor *auth.User) error {
	var notifications []Notification

	// 1. Mention notifications
	for _, mentionedUserID := range comment.Mentions {
		// Skip self-mention
		if mentionedUserID == actor.ID {
			continue
		}

		// Check if Recipient has blocked Actor
		if s.isBlocked(ctx, mentionedUserID, actor.ID) {
			continue
		}

		notifications = append(notifications, Notification{
			RecipientID:  mentionedUserID,
			ActorID:      actor.ID,
			Type:         TypeMention,
			ResourceType: "comment",
			ResourceID:   comment.ID,
			AnchorID:     &comment.AnchorID,
			Preview:      truncate(comment.Content, 100),
		})
	}

	// 2. Comment notification to anchor owner
	// Skip if:
	// - Commenter is anchor owner (self-comment)
	// - Anchor owner was already mentioned (avoid duplicate)
	if anchorUserID != actor.ID && !containsID(comment.Mentions, anchorUserID) {
		// Check if Owner has blocked Actor
		if !s.isBlocked(ctx, anchorUserID, actor.ID) {
			notifications = append(notifications, Notification{
				RecipientID:  anchorUserID,
				ActorID:      actor.ID,
				Type:         TypeComment,
				ResourceType: "anchor",
				ResourceID:   anchorID,
				AnchorID:     &anchorID,
				Preview:      truncate(comment.Content, 100),
			})
		}
	}

	// Batch insert
	if len(notifications) > 0 {
		return s.repo.CreateMany(ctx, notifications)
	}

	return nil
}

// CreateReplyNotifications notifies on a reply to a comment:
//   - mentioned users get a "mention"
//   - the parent comment's author gets a "reply"
//   - the anchor owner gets a "comment" (unless they are already notified above)
func (s *Service) CreateReplyNotifications(ctx context.Context, reply *CommentData, parentAuthorID, anchorUserID primitive.ObjectID, actor *auth.User) error {
	var out []Notification
	notified := map[primitive.ObjectID]bool{actor.ID: true} // never notify yourself

	add := func(recipient primitive.ObjectID, typ, resourceType string, resourceID primitive.ObjectID) {
		if notified[recipient] || s.isBlocked(ctx, recipient, actor.ID) {
			return
		}
		notified[recipient] = true
		out = append(out, Notification{
			RecipientID:  recipient,
			ActorID:      actor.ID,
			Type:         typ,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			AnchorID:     &reply.AnchorID,
			Preview:      truncate(reply.Content, 100),
		})
	}

	for _, mentioned := range reply.Mentions {
		add(mentioned, TypeMention, "comment", reply.ID)
	}
	add(parentAuthorID, TypeReply, "comment", reply.ID)
	add(anchorUserID, TypeComment, "anchor", reply.AnchorID)

	if len(out) == 0 {
		return nil
	}
	return s.repo.CreateMany(ctx, out)
}

// CreateEditCommentNotifications creates notifications for NEW mentions in edited comment
func (s *Service) CreateEditCommentNotifications(ctx context.Context, comment *CommentData, oldMentions []primitive.ObjectID, actor *auth.User) error {
	// Find new mentions (in current but not in old)
	oldMentionSet := make(map[primitive.ObjectID]bool)
	for _, id := range oldMentions {
		oldMentionSet[id] = true
	}

	var newMentions []primitive.ObjectID
	for _, id := range comment.Mentions {
		if !oldMentionSet[id] {
			newMentions = append(newMentions, id)
		}
	}

	if len(newMentions) == 0 {
		return nil
	}

	var notifications []Notification
	for _, mentionedUserID := range newMentions {
		// Skip self-mention
		if mentionedUserID == actor.ID {
			continue
		}

		// Check if Recipient has blocked Actor
		if s.isBlocked(ctx, mentionedUserID, actor.ID) {
			continue
		}

		notifications = append(notifications, Notification{
			RecipientID:  mentionedUserID,
			ActorID:      actor.ID,
			Type:         TypeMention,
			ResourceType: "comment",
			ResourceID:   comment.ID,
			AnchorID:     &comment.AnchorID,
			Preview:      truncate(comment.Content, 100),
		})
	}

	if len(notifications) > 0 {
		return s.repo.CreateMany(ctx, notifications)
	}

	return nil
}

// Helper functions

func truncate(s string, maxLen int) string {
	// Count characters, not bytes, so multi-byte characters are never cut in half
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen-3]) + "..."
}

func containsID(ids []primitive.ObjectID, id primitive.ObjectID) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

// CreateLikeNotification creates notification when someone likes an anchor
func (s *Service) CreateLikeNotification(ctx context.Context, anchorID primitive.ObjectID, anchorTitle string, actorID, ownerID primitive.ObjectID) error {
	// No self-notification
	if actorID == ownerID {
		return nil
	}

	// Check if Owner has blocked Actor
	if s.isBlocked(ctx, ownerID, actorID) {
		return nil
	}

	// One notification per actor+target: toggling like/unlike or follow/unfollow repeatedly
	// must not spam the recipient
	if exists, err := s.repo.Exists(ctx, ownerID, actorID, TypeLike, anchorID); err != nil || exists {
		return err
	}

	notification := Notification{
		RecipientID:  ownerID,
		ActorID:      actorID,
		Type:         TypeLike,
		ResourceType: "anchor",
		ResourceID:   anchorID,
		AnchorID:     &anchorID,
		Preview:      truncate(anchorTitle, 100),
	}

	return s.repo.CreateNotification(ctx, &notification)
}

// CreateFollowNotification creates notification when someone follows a user
func (s *Service) CreateFollowNotification(ctx context.Context, actorID, targetUserID primitive.ObjectID) error {
	// No self-notification
	if actorID == targetUserID {
		return nil
	}

	// Check if Target (Recipient) has blocked Actor
	if s.isBlocked(ctx, targetUserID, actorID) {
		return nil
	}

	// One notification per actor+target: toggling like/unlike or follow/unfollow repeatedly
	// must not spam the recipient
	if exists, err := s.repo.Exists(ctx, targetUserID, actorID, TypeFollow, actorID); err != nil || exists {
		return err
	}

	notification := Notification{
		RecipientID:  targetUserID,
		ActorID:      actorID,
		Type:         TypeFollow,
		ResourceType: "user",
		ResourceID:   actorID, // The follower is the resource
		AnchorID:     nil,
		Preview:      "",
	}

	return s.repo.CreateNotification(ctx, &notification)
}

// CreateCloneNotification creates notification when someone clones an anchor
func (s *Service) CreateCloneNotification(ctx context.Context, clonedAnchorID, originalAnchorID primitive.ObjectID, anchorTitle string, actorID, ownerID primitive.ObjectID) error {
	// No self-notification
	if actorID == ownerID {
		return nil
	}

	// Check if Owner has blocked Actor
	if s.isBlocked(ctx, ownerID, actorID) {
		return nil
	}

	notification := Notification{
		RecipientID:  ownerID,
		ActorID:      actorID,
		Type:         TypeClone,
		ResourceType: "anchor",
		ResourceID:   clonedAnchorID, // The new cloned anchor
		AnchorID:     &originalAnchorID,
		Preview:      truncate(anchorTitle, 100),
	}

	return s.repo.CreateNotification(ctx, &notification)
}

// isBlocked reports whether the recipient has blocked or muted the actor; either way the
// recipient should not get notifications about the actor's activity.
func (s *Service) isBlocked(ctx context.Context, recipientID, actorID primitive.ObjectID) bool {
	user, err := s.authRepo.GetUserByObjectID(ctx, recipientID)
	if err != nil || user == nil {
		return false // Default to not blocked if error
	}
	return user.HasBlocked(actorID) || user.HasMuted(actorID)
}

// CreateAnchorUpdateNotifications notifies followers about anchor updates
func (s *Service) CreateAnchorUpdateNotifications(ctx context.Context, anchorID primitive.ObjectID, anchorTitle string, authorID primitive.ObjectID) error {
	if s.followerProvider == nil {
		return nil
	}

	// Get all follower IDs with notifyOnUpdate=true
	followerIDs, err := s.followerProvider.GetFollowersWithNotifications(ctx, anchorID)
	if err != nil {
		return err
	}

	if len(followerIDs) == 0 {
		return nil
	}

	// Create notifications in batch
	now := time.Now()
	notificationsList := make([]Notification, 0)

	for _, followerID := range followerIDs {
		// Don't notify the author, or followers who blocked or muted the author
		if followerID == authorID || s.isBlocked(ctx, followerID, authorID) {
			continue
		}

		notification := Notification{
			ID:           primitive.NewObjectID(),
			RecipientID:  followerID,
			ActorID:      authorID,
			Type:         TypeAnchorUpdate,
			ResourceType: "anchor",
			ResourceID:   anchorID,
			AnchorID:     &anchorID,
			Preview:      truncateString(anchorTitle, 100),
			IsRead:       false,
			CreatedAt:    now,
		}
		notificationsList = append(notificationsList, notification)
	}

	if len(notificationsList) == 0 {
		return nil
	}

	return s.repo.CreateNotifications(ctx, notificationsList)
}

// Helper function
func truncateString(s string, maxLen int) string {
	// Count characters, not bytes, so multi-byte characters are never cut in half
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen-3]) + "..."
}
