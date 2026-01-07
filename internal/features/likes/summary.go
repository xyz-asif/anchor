package likes

import (
	"context"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GetLikeSummaryStandalone is a standalone function to get like summary without handler dependencies
// This avoids import cycles while still providing the functionality
func GetLikeSummaryStandalone(
	ctx context.Context,
	anchorID primitive.ObjectID,
	anchorLikeCount int,
	currentUserID *primitive.ObjectID,
	likesRepo *Repository,
	authRepoGetter func([]primitive.ObjectID) ([]interface{}, error),
	followsRepoGetter func(primitive.ObjectID, []primitive.ObjectID) (map[primitive.ObjectID]bool, error),
) (*LikeSummaryResponse, error) {

	// If no likes, return empty summary
	if anchorLikeCount == 0 {
		return &LikeSummaryResponse{
			TotalCount:       0,
			HasLiked:         false,
			LikedByFollowing: []LikeSummaryUser{},
			OtherLikersCount: 0,
		}, nil
	}

	// Check if current user has liked (if authenticated)
	hasLiked := false
	if currentUserID != nil {
		hasLiked, _ = likesRepo.ExistsLike(ctx, anchorID, *currentUserID)
	}

	// Get recent likers (limit 20 for processing)
	recentLikes, err := likesRepo.GetRecentLikers(ctx, anchorID, 20)
	if err != nil {
		return nil, err
	}

	// Extract liker IDs
	var likerIDs []primitive.ObjectID
	for _, like := range recentLikes {
		likerIDs = append(likerIDs, like.UserID)
	}

	var prioritizedIDs []primitive.ObjectID

	if currentUserID == nil {
		// Not authenticated - just take first 3
		if len(likerIDs) > 3 {
			prioritizedIDs = likerIDs[:3]
		} else {
			prioritizedIDs = likerIDs
		}
	} else {
		// Authenticated - prioritize followed users
		followingMap, _ := followsRepoGetter(*currentUserID, likerIDs)

		// Separate into followed and not followed
		var followedLikerIDs []primitive.ObjectID
		var otherLikerIDs []primitive.ObjectID

		for _, id := range likerIDs {
			if followingMap[id] {
				followedLikerIDs = append(followedLikerIDs, id)
			} else {
				otherLikerIDs = append(otherLikerIDs, id)
			}
		}

		// Prioritize: followed first, then others (max 3 total)
		prioritizedIDs = append(followedLikerIDs, otherLikerIDs...)
		if len(prioritizedIDs) > 3 {
			prioritizedIDs = prioritizedIDs[:3]
		}
	}

	// Fetch user details for prioritized users
	usersInterface, err := authRepoGetter(prioritizedIDs)
	if err != nil {
		return nil, err
	}

	// Build likedByFollowing list
	var likedByFollowing []LikeSummaryUser
	for _, userInterface := range usersInterface {
		// Type assert to extract user fields
		if userMap, ok := userInterface.(map[string]interface{}); ok {
			user := LikeSummaryUser{
				ID:          userMap["id"].(primitive.ObjectID),
				Username:    userMap["username"].(string),
				DisplayName: userMap["displayName"].(string),
			}
			if profilePic, ok := userMap["profilePicture"].(*string); ok {
				user.ProfilePicture = profilePic
			}
			likedByFollowing = append(likedByFollowing, user)
		}
	}

	otherLikersCount := anchorLikeCount - len(likedByFollowing)
	if otherLikersCount < 0 {
		otherLikersCount = 0
	}

	return &LikeSummaryResponse{
		TotalCount:       anchorLikeCount,
		HasLiked:         hasLiked,
		LikedByFollowing: likedByFollowing,
		OtherLikersCount: otherLikersCount,
	}, nil
}
