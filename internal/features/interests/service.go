package interests

import (
	"context"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// GetSuggestedInterests returns personalized interest categories
func (s *Service) GetSuggestedInterests(ctx context.Context, userID *primitive.ObjectID, limit int) (*SuggestedInterestsResponse, error) {
	// If not authenticated, return popular categories
	if userID == nil {
		return s.getPopularCategories(ctx, limit)
	}

	// Get user's tags from different sources
	ownTags, _ := s.repo.GetUserOwnAnchorTags(ctx, *userID)
	likedTags, _ := s.repo.GetUserLikedAnchorTags(ctx, *userID)
	followedTags, _ := s.repo.GetUserFollowedAnchorTags(ctx, *userID)

	// If user has no activity, return popular categories
	if len(ownTags) == 0 && len(likedTags) == 0 && len(followedTags) == 0 {
		return s.getPopularCategories(ctx, limit)
	}

	// Calculate weighted scores for tags
	tagScores := make(map[string]float64)

	// Own anchors: weight 3
	for i, tag := range ownTags {
		weight := 3.0 * (1.0 - float64(i)*0.05)
		if weight < 0.5 {
			weight = 0.5
		}
		tagScores[tag] += weight
	}

	// Liked anchors: weight 2
	for i, tag := range likedTags {
		weight := 2.0 * (1.0 - float64(i)*0.05)
		if weight < 0.3 {
			weight = 0.3
		}
		tagScores[tag] += weight
	}

	// Followed anchors: weight 2
	for i, tag := range followedTags {
		weight := 2.0 * (1.0 - float64(i)*0.05)
		if weight < 0.3 {
			weight = 0.3
		}
		tagScores[tag] += weight
	}

	// Sort tags by score
	type tagScore struct {
		Tag   string
		Score float64
	}

	sortedTags := make([]tagScore, 0, len(tagScores))
	for tag, score := range tagScores {
		sortedTags = append(sortedTags, tagScore{Tag: tag, Score: score})
	}

	sort.Slice(sortedTags, func(i, j int) bool {
		return sortedTags[i].Score > sortedTags[j].Score
	})

	// Take top tags
	topTags := make([]string, 0)
	for i, ts := range sortedTags {
		if i >= limit*2 {
			break
		}
		topTags = append(topTags, ts.Tag)
	}

	// Get anchor counts for these tags
	tagCounts, _ := s.repo.GetTagAnchorCounts(ctx, topTags)

	// Find max score for normalization
	maxScore := 0.0
	for _, ts := range sortedTags {
		if ts.Score > maxScore {
			maxScore = ts.Score
		}
	}
	if maxScore == 0 {
		maxScore = 1
	}

	// Build categories
	categories := make([]Category, 0)
	for _, ts := range sortedTags {
		if len(categories) >= limit {
			break
		}

		count := tagCounts[ts.Tag]
		if count == 0 {
			continue
		}

		relevance := ts.Score / maxScore

		categories = append(categories, Category{
			Name:           ts.Tag,
			DisplayName:    capitalizeFirst(ts.Tag),
			AnchorCount:    count,
			RelevanceScore: relevance,
		})
	}

	// If not enough categories, add popular ones
	if len(categories) < limit {
		popular, _ := s.repo.GetPopularCategories(ctx, limit)

		existingTags := make(map[string]bool)
		for _, c := range categories {
			existingTags[c.Name] = true
		}

		for _, p := range popular {
			if len(categories) >= limit {
				break
			}
			if !existingTags[p.Name] {
				categories = append(categories, Category{
					Name:           p.Name,
					DisplayName:    capitalizeFirst(p.Name),
					AnchorCount:    p.Count,
					RelevanceScore: 0.3,
				})
			}
		}
	}

	// Limit tags for basedOn
	limitTags := func(tags []string, max int) []string {
		if len(tags) > max {
			return tags[:max]
		}
		return tags
	}

	return &SuggestedInterestsResponse{
		Categories:   categories,
		Personalized: true,
		BasedOn: &BasedOn{
			OwnAnchorTags:      limitTags(ownTags, 5),
			LikedAnchorTags:    limitTags(likedTags, 5),
			FollowedAnchorTags: limitTags(followedTags, 5),
		},
	}, nil
}

func (s *Service) getPopularCategories(ctx context.Context, limit int) (*SuggestedInterestsResponse, error) {
	popular, err := s.repo.GetPopularCategories(ctx, limit)
	if err != nil {
		return nil, err
	}

	maxCount := 0
	for _, p := range popular {
		if p.Count > maxCount {
			maxCount = p.Count
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}

	categories := make([]Category, len(popular))
	for i, p := range popular {
		categories[i] = Category{
			Name:           p.Name,
			DisplayName:    capitalizeFirst(p.Name),
			AnchorCount:    p.Count,
			RelevanceScore: float64(p.Count) / float64(maxCount),
		}
	}

	return &SuggestedInterestsResponse{
		Categories:   categories,
		Personalized: false,
		BasedOn:      nil,
	}, nil
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(string(s[0])) + s[1:]
}
