package routes

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// cleanupUserActivity removes everything a deleted user left on other people's content and
// recomputes the counters it affected (likes, comments, replies, followers, following).
// Counts are recomputed from the source collections instead of decremented, so they end up
// correct even if they had drifted before.
func cleanupUserActivity(ctx context.Context, db *mongo.Database, userID primitive.ObjectID) error {
	anchorsColl := db.Collection("anchors")
	usersColl := db.Collection("users")
	commentsColl := db.Collection("comments")

	// 1. Likes the user gave -> recompute likeCount on those anchors
	likedAnchors, err := distinctIDs(ctx, db.Collection("likes"), "anchorId", bson.M{"userId": userID})
	if err != nil {
		return err
	}
	if _, err := db.Collection("likes").DeleteMany(ctx, bson.M{"userId": userID}); err != nil {
		return err
	}

	// 2. Comment likes the user gave -> recompute likeCount on those comments
	likedComments, err := distinctIDs(ctx, db.Collection("commentLikes"), "commentId", bson.M{"userId": userID})
	if err != nil {
		return err
	}
	if _, err := db.Collection("commentLikes").DeleteMany(ctx, bson.M{"userId": userID}); err != nil {
		return err
	}
	for _, id := range likedComments {
		n, err := db.Collection("commentLikes").CountDocuments(ctx, bson.M{"commentId": id})
		if err == nil {
			_, _ = commentsColl.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"likeCount": n}})
		}
	}

	// 3. The user's comments (and replies to their top-level comments) are soft-deleted
	//    -> recompute commentCount on affected anchors and replyCount on affected parents
	userCommentFilter := bson.M{"userId": userID, "deletedAt": nil}
	commentedAnchors, err := distinctIDs(ctx, commentsColl, "anchorId", userCommentFilter)
	if err != nil {
		return err
	}
	parents, err := distinctIDs(ctx, commentsColl, "parentId", bson.M{"userId": userID, "deletedAt": nil, "parentId": bson.M{"$ne": nil}})
	if err != nil {
		return err
	}
	topLevel, err := distinctIDs(ctx, commentsColl, "_id", bson.M{"userId": userID, "deletedAt": nil, "parentId": nil})
	if err != nil {
		return err
	}
	now := time.Now()
	softDelete := bson.M{"$set": bson.M{"deletedAt": now, "updatedAt": now}}
	if _, err := commentsColl.UpdateMany(ctx, userCommentFilter, softDelete); err != nil {
		return err
	}
	if len(topLevel) > 0 {
		if _, err := commentsColl.UpdateMany(ctx, bson.M{"parentId": bson.M{"$in": topLevel}, "deletedAt": nil}, softDelete); err != nil {
			return err
		}
	}
	for _, id := range parents {
		n, err := commentsColl.CountDocuments(ctx, bson.M{"parentId": id, "deletedAt": nil})
		if err == nil {
			_, _ = commentsColl.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"replyCount": n}})
		}
	}

	// 4. Recompute anchor counters touched by steps 1 and 3
	for _, id := range likedAnchors {
		n, err := db.Collection("likes").CountDocuments(ctx, bson.M{"anchorId": id})
		if err == nil {
			_, _ = anchorsColl.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"likeCount": n}})
		}
	}
	for _, id := range commentedAnchors {
		n, err := commentsColl.CountDocuments(ctx, bson.M{"anchorId": id, "deletedAt": nil})
		if err == nil {
			_, _ = anchorsColl.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"commentCount": n}})
		}
	}

	// 5. Follows in both directions -> recompute the other side's follower/following counts
	followsColl := db.Collection("follows")
	followed, err := distinctIDs(ctx, followsColl, "followingId", bson.M{"followerId": userID})
	if err != nil {
		return err
	}
	followers, err := distinctIDs(ctx, followsColl, "followerId", bson.M{"followingId": userID})
	if err != nil {
		return err
	}
	if _, err := followsColl.DeleteMany(ctx, bson.M{"$or": []bson.M{{"followerId": userID}, {"followingId": userID}}}); err != nil {
		return err
	}
	for _, id := range followed {
		n, err := followsColl.CountDocuments(ctx, bson.M{"followingId": id})
		if err == nil {
			_, _ = usersColl.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"followerCount": n}})
		}
	}
	for _, id := range followers {
		n, err := followsColl.CountDocuments(ctx, bson.M{"followerId": id})
		if err == nil {
			_, _ = usersColl.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"followingCount": n}})
		}
	}

	// 6. Anchors the user followed -> recompute their followerCount
	anchorFollowsColl := db.Collection("anchor_follows")
	followedAnchors, err := distinctIDs(ctx, anchorFollowsColl, "anchorId", bson.M{"userId": userID})
	if err != nil {
		return err
	}
	if _, err := anchorFollowsColl.DeleteMany(ctx, bson.M{"userId": userID}); err != nil {
		return err
	}
	for _, id := range followedAnchors {
		n, err := anchorFollowsColl.CountDocuments(ctx, bson.M{"anchorId": id})
		if err == nil {
			_, _ = anchorsColl.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"followerCount": n}})
		}
	}

	// 7. Notifications to or from the user, reports filed by or about the user
	if _, err := db.Collection("notifications").DeleteMany(ctx, bson.M{"$or": []bson.M{{"recipientId": userID}, {"actorId": userID}}}); err != nil {
		return err
	}
	if _, err := db.Collection("reports").DeleteMany(ctx, bson.M{"$or": []bson.M{{"reporterId": userID}, {"targetId": userID}}}); err != nil {
		return err
	}

	// 8. Remove the user from other people's block and mute lists
	// (filter on the id so documents with null lists are never touched: $pull fails on null)
	if _, err := usersColl.UpdateMany(ctx, bson.M{"blockedUsers": userID}, bson.M{"$pull": bson.M{"blockedUsers": userID}}); err != nil {
		return err
	}
	if _, err := usersColl.UpdateMany(ctx, bson.M{"mutedUsers": userID}, bson.M{"$pull": bson.M{"mutedUsers": userID}}); err != nil {
		return err
	}

	log.Printf("Account cleanup for %s: %d likes, %d followed users, %d followers, %d followed anchors",
		userID.Hex(), len(likedAnchors), len(followed), len(followers), len(followedAnchors))
	return nil
}

// distinctIDs returns the distinct ObjectID values of field for documents matching filter
func distinctIDs(ctx context.Context, coll *mongo.Collection, field string, filter bson.M) ([]primitive.ObjectID, error) {
	values, err := coll.Distinct(ctx, field, filter)
	if err != nil {
		return nil, err
	}
	ids := make([]primitive.ObjectID, 0, len(values))
	for _, v := range values {
		if id, ok := v.(primitive.ObjectID); ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// assetSharedByOthers reports whether items outside this anchor reference the same media URL
// (clones point at the original's files), in which case the file must not be deleted.
func assetSharedByOthers(ctx context.Context, db *mongo.Database, anchorID primitive.ObjectID, url string) bool {
	if url == "" {
		return false
	}
	n, err := db.Collection("items").CountDocuments(ctx, bson.M{
		"anchorId": bson.M{"$ne": anchorID},
		"$or": []bson.M{
			{"imageData.cloudinaryUrl": url},
			{"audioData.cloudinaryUrl": url},
			{"fileData.cloudinaryUrl": url},
		},
	})
	// On error, keep the file: a leftover file is cheaper than breaking someone's clone
	return err != nil || n > 0
}
