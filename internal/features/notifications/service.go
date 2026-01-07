package notifications

import (
	"context"

	"github.com/xyz-asif/gotodo/internal/features/auth"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Service struct {
	repo     *Repository
	authRepo *auth.Repository
}

func NewService(repo *Repository, authRepo *auth.Repository) *Service {
	return &Service{
		repo:     repo,
		authRepo: authRepo,
	}
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

	// Batch insert
	if len(notifications) > 0 {
		return s.repo.CreateMany(ctx, notifications)
	}

	return nil
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
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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
