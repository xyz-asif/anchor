package interests

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Repository struct {
	userCollection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		userCollection: db.Collection("users"),
	}
}

// ErrUserNotFound is returned when the user no longer exists (e.g. deleted meanwhile)
var ErrUserNotFound = errors.New("user not found")

// maxLikesConsidered bounds the personalised suggestions to the user's most recent likes
const maxLikesConsidered = 500

// UpdateUserInterests replaces the user's interests (an empty list clears them)
func (r *Repository) UpdateUserInterests(ctx context.Context, userID primitive.ObjectID, tags []string) error {
	result, err := r.userCollection.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"interests": tags,
			"updatedAt": time.Now(),
		},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrUserNotFound
	}
	return nil
}

// tagCount is one tag with how often it appears
type tagCount struct {
	Name  string `bson:"_id"`
	Count int    `bson:"count"`
}

// GetSuggestedTags ranks tags from the user's own anchors (weight 3) and from anchors they
// recently liked (weight 2). Liked anchors that were deleted or made private don't count, so
// a private anchor's tags never surface in someone else's suggestions. Tags are compared
// case-insensitively, and ties are broken by name so the order is the same on every request.
func (r *Repository) GetSuggestedTags(ctx context.Context, userID primitive.ObjectID, limit int) ([]Category, error) {
	db := r.userCollection.Database()

	ownPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": userID, "deletedAt": nil}}},
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$group", Value: bson.M{"_id": bson.M{"$toLower": "$tags"}, "count": bson.M{"$sum": 1}}}},
	}
	var own []tagCount
	if err := aggregate(ctx, db.Collection("anchors"), ownPipeline, &own); err != nil {
		return nil, err
	}

	likedPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": userID}}},
		{{Key: "$sort", Value: bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}}},
		{{Key: "$limit", Value: maxLikesConsidered}},
		{{Key: "$lookup", Value: bson.M{
			"from": "anchors",
			"let":  bson.M{"aid": "$anchorId"},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{"$expr": bson.M{"$eq": bson.A{"$_id", "$$aid"}}}},
				bson.M{"$match": bson.M{"deletedAt": nil, "$or": bson.A{
					bson.M{"visibility": bson.M{"$ne": "private"}},
					bson.M{"userId": userID},
				}}},
				bson.M{"$project": bson.M{"tags": 1}},
			},
			"as": "anchor",
		}}},
		{{Key: "$unwind", Value: "$anchor"}},
		{{Key: "$unwind", Value: "$anchor.tags"}},
		{{Key: "$group", Value: bson.M{"_id": bson.M{"$toLower": "$anchor.tags"}, "count": bson.M{"$sum": 1}}}},
	}
	var liked []tagCount
	if err := aggregate(ctx, db.Collection("likes"), likedPipeline, &liked); err != nil {
		return nil, err
	}

	byName := map[string]*Category{}
	add := func(list []tagCount, weight int) {
		for _, t := range list {
			if t.Name == "" {
				continue
			}
			cat, ok := byName[t.Name]
			if !ok {
				cat = &Category{Name: t.Name}
				byName[t.Name] = cat
			}
			cat.Count += t.Count
			cat.Score += t.Count * weight
		}
	}
	add(own, 3)
	add(liked, 2)

	result := make([]Category, 0, len(byName))
	for _, cat := range byName {
		result = append(result, *cat)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Name < result[j].Name
	})
	if len(result) > limit {
		result = result[:limit]
	}
	for i := range result {
		result[i].Slug = generateSlug(result[i].Name)
		result[i].Icon = getIconForCategory(result[i].Name)
	}
	return result, nil
}

// GetPopularTags returns the most used tags on public anchors. While the platform is young and
// has fewer tags than requested, the list is topped up with starter categories, so an interest
// picker is never nearly empty.
func (r *Repository) GetPopularTags(ctx context.Context, limit int) ([]Category, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"deletedAt": nil, "visibility": "public"}}},
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$group", Value: bson.M{"_id": bson.M{"$toLower": "$tags"}, "count": bson.M{"$sum": 1}}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: limit}},
	}
	var counts []tagCount
	if err := aggregate(ctx, r.userCollection.Database().Collection("anchors"), pipeline, &counts); err != nil {
		return nil, err
	}

	result := make([]Category, 0, limit)
	seen := map[string]bool{}
	for _, t := range counts {
		if t.Name == "" {
			continue
		}
		seen[t.Name] = true
		result = append(result, Category{Name: t.Name, Slug: generateSlug(t.Name), Icon: getIconForCategory(t.Name), Count: t.Count, Score: t.Count})
	}
	for _, seed := range getSeedCategories(len(seedCategories)) {
		if len(result) >= limit {
			break
		}
		if !seen[seed.Slug] {
			seen[seed.Slug] = true
			result = append(result, seed)
		}
	}
	return result, nil
}

func aggregate(ctx context.Context, coll *mongo.Collection, pipeline mongo.Pipeline, out interface{}) error {
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	return cursor.All(ctx, out)
}

// seedCategories are starter interests for a platform without many tags yet. Names use the
// stored tag format (lowercase), so a picked seed matches anchor tags exactly.
var seedCategories = []string{"tech", "photography", "travel", "food", "fitness", "music", "art", "books", "gaming", "fashion", "design", "business", "science", "sports", "movies", "nature", "cooking", "diy"}

// getSeedCategories returns up to limit starter categories
func getSeedCategories(limit int) []Category {
	if limit > len(seedCategories) {
		limit = len(seedCategories)
	}
	out := make([]Category, limit)
	for i, name := range seedCategories[:limit] {
		out[i] = Category{Name: name, Slug: name, Icon: getIconForCategory(name)}
	}
	return out
}

// generateSlug creates a URL-friendly slug from a tag name
func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	return slug
}

// getIconForCategory returns an emoji icon for a category
func getIconForCategory(name string) string {
	iconMap := map[string]string{
		"tech":        "💻",
		"photography": "📷",
		"travel":      "✈️",
		"food":        "🍔",
		"fitness":     "💪",
		"music":       "🎵",
		"art":         "🎨",
		"books":       "📚",
		"gaming":      "🎮",
		"fashion":     "👗",
		"design":      "✨",
		"business":    "💼",
		"science":     "🔬",
		"sports":      "⚽",
		"movies":      "🎬",
		"nature":      "🌿",
		"cooking":     "👨‍🍳",
		"diy":         "🔨",
	}

	slug := generateSlug(name)
	if icon, ok := iconMap[slug]; ok {
		return icon
	}
	return "🏷️" // Default tag icon
}
