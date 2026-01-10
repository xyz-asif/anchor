package interests

import (
	"context"
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

// UpdateUserInterests updates the interests tags for a user
func (r *Repository) UpdateUserInterests(ctx context.Context, userID primitive.ObjectID, tags []string) error {
	filter := bson.M{"_id": userID}
	update := bson.M{
		"$set": bson.M{
			"interests": tags,
			"updatedAt": time.Now(),
		},
	}
	_, err := r.userCollection.UpdateOne(ctx, filter, update)
	return err
}

// GetSuggestedTags aggregates tags from user's own anchors and liked anchors
func (r *Repository) GetSuggestedTags(ctx context.Context, userID primitive.ObjectID, limit int) ([]Category, error) {
	anchorsCollection := r.userCollection.Database().Collection("anchors")
	likesCollection := r.userCollection.Database().Collection("likes")

	// 1. Get tags from user's own anchors
	ownPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": userID, "deletedAt": nil}}},
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$tags",
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$project", Value: bson.M{
			"name":  "$_id",
			"count": 1,
			"score": bson.M{"$multiply": []interface{}{"$count", 3}}, // Own anchors weighted 3x
		}}},
	}

	ownCursor, err := anchorsCollection.Aggregate(ctx, ownPipeline)
	if err != nil {
		return nil, err
	}
	defer ownCursor.Close(ctx)

	var ownTags []Category
	if err = ownCursor.All(ctx, &ownTags); err != nil {
		return nil, err
	}

	// 2. Get tags from liked anchors
	likedPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": userID}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "anchors",
			"localField":   "anchorId",
			"foreignField": "_id",
			"as":           "anchor",
		}}},
		{{Key: "$unwind", Value: "$anchor"}},
		{{Key: "$unwind", Value: "$anchor.tags"}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$anchor.tags",
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$project", Value: bson.M{
			"name":  "$_id",
			"count": 1,
			"score": bson.M{"$multiply": []interface{}{"$count", 2}}, // Liked anchors weighted 2x
		}}},
	}

	likedCursor, err := likesCollection.Aggregate(ctx, likedPipeline)
	if err != nil {
		return nil, err
	}
	defer likedCursor.Close(ctx)

	var likedTags []Category
	if err = likedCursor.All(ctx, &likedTags); err != nil {
		return nil, err
	}

	// 3. Merge and deduplicate
	tagMap := make(map[string]*Category)
	for _, tag := range ownTags {
		tagMap[tag.Name] = &tag
	}
	for _, tag := range likedTags {
		if existing, ok := tagMap[tag.Name]; ok {
			existing.Count += tag.Count
			existing.Score += tag.Score
		} else {
			tagMap[tag.Name] = &tag
		}
	}

	// Convert to slice and sort by score
	result := make([]Category, 0, len(tagMap))
	for _, cat := range tagMap {
		result = append(result, *cat)
	}

	// Sort by score descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	if len(result) > limit {
		result = result[:limit]
	}

	// Populate slug and icon for each tag
	for i := range result {
		result[i].Slug = generateSlug(result[i].Name)
		result[i].Icon = getIconForCategory(result[i].Name)
	}

	return result, nil
}

// GetPopularTags returns the most popular tags across the platform
func (r *Repository) GetPopularTags(ctx context.Context, limit int) ([]Category, error) {
	anchorsCollection := r.userCollection.Database().Collection("anchors")

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"deletedAt": nil, "visibility": "public"}}},
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$tags",
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.M{"count": -1}}},
		{{Key: "$limit", Value: limit}},
		{{Key: "$project", Value: bson.M{
			"name":  "$_id",
			"count": 1,
			"score": "$count",
		}}},
	}

	cursor, err := anchorsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var tags []Category
	if err = cursor.All(ctx, &tags); err != nil {
		return nil, err
	}

	// If no tags found, return seed data
	if len(tags) == 0 {
		return getSeedCategories(limit), nil
	}

	// Populate slug and icon for each tag
	for i := range tags {
		tags[i].Slug = generateSlug(tags[i].Name)
		tags[i].Icon = getIconForCategory(tags[i].Name)
	}

	return tags, nil
}

// getSeedCategories returns hardcoded popular categories for onboarding
func getSeedCategories(limit int) []Category {
	seeds := []Category{
		{Name: "Tech", Slug: "tech", Icon: "💻", Count: 0, Score: 0},
		{Name: "Photography", Slug: "photography", Icon: "📷", Count: 0, Score: 0},
		{Name: "Travel", Slug: "travel", Icon: "✈️", Count: 0, Score: 0},
		{Name: "Food", Slug: "food", Icon: "🍔", Count: 0, Score: 0},
		{Name: "Fitness", Slug: "fitness", Icon: "💪", Count: 0, Score: 0},
		{Name: "Music", Slug: "music", Icon: "🎵", Count: 0, Score: 0},
		{Name: "Art", Slug: "art", Icon: "🎨", Count: 0, Score: 0},
		{Name: "Books", Slug: "books", Icon: "📚", Count: 0, Score: 0},
		{Name: "Gaming", Slug: "gaming", Icon: "🎮", Count: 0, Score: 0},
		{Name: "Fashion", Slug: "fashion", Icon: "👗", Count: 0, Score: 0},
		{Name: "Design", Slug: "design", Icon: "✨", Count: 0, Score: 0},
		{Name: "Business", Slug: "business", Icon: "💼", Count: 0, Score: 0},
		{Name: "Science", Slug: "science", Icon: "🔬", Count: 0, Score: 0},
		{Name: "Sports", Slug: "sports", Icon: "⚽", Count: 0, Score: 0},
		{Name: "Movies", Slug: "movies", Icon: "🎬", Count: 0, Score: 0},
		{Name: "Nature", Slug: "nature", Icon: "🌿", Count: 0, Score: 0},
		{Name: "Cooking", Slug: "cooking", Icon: "👨‍🍳", Count: 0, Score: 0},
		{Name: "DIY", Slug: "diy", Icon: "🔨", Count: 0, Score: 0},
	}

	if limit > len(seeds) {
		limit = len(seeds)
	}
	return seeds[:limit]
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
