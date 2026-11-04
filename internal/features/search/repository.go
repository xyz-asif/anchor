package search

import (
	"context"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/xyz-asif/gotodo/internal/pkg/searchterms"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Repository struct {
	anchorsCollection *mongo.Collection
	usersCollection   *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	r := &Repository{
		anchorsCollection: db.Collection("anchors"),
		usersCollection:   db.Collection("users"),
	}
	// Index anchors and users saved before search terms existed (or with an older version of
	// them). Runs in the background; new and edited documents are indexed as they're written.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := r.Backfill(ctx); err != nil {
			log.Printf("search: indexing existing anchors and users failed: %v", err)
		}
	}()
	return r
}

// usernamePrefixPattern matches what can start a username (usernames are lowercase a-z, 0-9, _)
var usernamePrefixPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// Match modes: every query word must match, or (when that finds nothing) any of them
const (
	matchAll = "$all"
	matchAny = "$in"
)

// sizeOfOverlap is the number of query terms found in a document's array field
func sizeOfOverlap(terms []string, field string) bson.M {
	return bson.M{"$size": bson.M{"$setIntersection": bson.A{terms, bson.M{"$ifNull": bson.A{field, bson.A{}}}}}}
}

// SearchAnchors finds public anchors containing every query word as the start of a word in the
// title, tags or description, so "garde tip" finds "Gardening Tips" while typing. If no anchor
// has every word, it falls back to anchors with any of them (most matched words first) and
// reports matchedAll=false. Anchors by hiddenAuthors (block relationships) are excluded in
// the query, so totals and pages agree.
func (r *Repository) SearchAnchors(ctx context.Context, query, tag, sort string, page, limit int, hiddenAuthors []primitive.ObjectID) (docs []AnchorSearchDoc, total int64, matchedAll bool, err error) {
	terms := searchterms.QueryTerms(query)
	if len(terms) == 0 {
		return []AnchorSearchDoc{}, 0, true, nil
	}
	base := bson.M{
		"visibility": "public",
		"deletedAt":  nil,
	}
	if tag != "" {
		base["tags"] = tag // already normalised exactly like stored tags
	}
	if len(hiddenAuthors) > 0 {
		base["userId"] = bson.M{"$nin": hiddenAuthors}
	}

	docs, total, err = r.searchAnchors(ctx, base, terms, matchAll, sort, page, limit)
	if err != nil || total > 0 || len(terms) == 1 {
		return docs, total, true, err
	}
	docs, total, err = r.searchAnchors(ctx, base, terms, matchAny, sort, page, limit)
	return docs, total, false, err
}

func (r *Repository) searchAnchors(ctx context.Context, base bson.M, terms []string, mode, sort string, page, limit int) ([]AnchorSearchDoc, int64, error) {
	match := bson.M{"search.terms": bson.M{mode: terms}}
	for k, v := range base {
		match[k] = v
	}

	// Relevance: words matched at all, then words matched in the title, then whole title words
	score := bson.M{"$add": bson.A{
		bson.M{"$multiply": bson.A{sizeOfOverlap(terms, "$search.terms"), 10}},
		bson.M{"$multiply": bson.A{sizeOfOverlap(terms, "$search.title"), 3}},
		bson.M{"$multiply": bson.A{sizeOfOverlap(terms, "$search.titleWords"), 2}},
	}}

	// Every sort ends with _id, so equal scores/dates can't repeat or skip results across pages
	var sortStage bson.D
	switch sort {
	case SortRecent:
		sortStage = bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}
	case SortPopular:
		sortStage = bson.D{{Key: "engagementScore", Value: -1}, {Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}
	default: // relevant; popularity breaks ties
		sortStage = bson.D{{Key: "relevance", Value: -1}, {Key: "engagementScore", Value: -1}, {Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$facet", Value: bson.M{
			"rows": bson.A{
				bson.M{"$addFields": bson.M{"relevance": score}},
				bson.M{"$sort": sortStage},
				bson.M{"$skip": int64(page-1) * int64(limit)},
				bson.M{"$limit": int64(limit)},
				bson.M{"$project": bson.M{"search": 0}},
			},
			"total": bson.A{bson.M{"$count": "n"}},
		}}},
	}
	var rows []AnchorSearchDoc
	total, err := r.aggregatePage(ctx, r.anchorsCollection, pipeline, &rows)
	return rows, total, err
}

// SearchUsers finds users by the start of their username ("asi" or "@asi" finds "asif") or of
// any word of their name, or by whole words of their bio. Exact username matches come first,
// then username prefixes, then name matches, then bio relevance, then popularity. If nobody
// matches every word, users matching any of them are returned and matchedAll is false.
func (r *Repository) SearchUsers(ctx context.Context, query string, page, limit int, hidden []primitive.ObjectID) (docs []UserSearchDoc, total int64, matchedAll bool, err error) {
	terms := searchterms.QueryTerms(query)
	if len(terms) == 0 {
		return []UserSearchDoc{}, 0, true, nil
	}
	docs, total, err = r.searchUsers(ctx, query, terms, matchAll, page, limit, hidden)
	if err != nil || total > 0 || len(terms) == 1 {
		return docs, total, true, err
	}
	docs, total, err = r.searchUsers(ctx, query, terms, matchAny, page, limit, hidden)
	return docs, total, false, err
}

func (r *Repository) searchUsers(ctx context.Context, query string, terms []string, mode string, page, limit int, hidden []primitive.ObjectID) ([]UserSearchDoc, int64, error) {
	clauses := bson.A{bson.M{"search.terms": bson.M{mode: terms}}}
	// Bio is searched by whole words; the words are passed without any $text operators
	// (a leading '-' would otherwise mean "exclude", quotes "exact phrase")
	useText := mode == matchAll
	if useText {
		clauses = append(clauses, bson.M{"$text": bson.M{"$search": strings.Join(terms, " ")}})
	}
	match := bson.M{"$or": clauses}
	if len(hidden) > 0 {
		match["_id"] = bson.M{"$nin": hidden}
	}

	rank := bson.M{
		"nameHits":  bson.M{"$add": bson.A{sizeOfOverlap(terms, "$search.title"), sizeOfOverlap(terms, "$search.titleWords")}},
		"wordHits":  sizeOfOverlap(terms, "$search.terms"),
		"exact":     false,
		"prefix":    false,
		"textScore": 0,
	}
	if useText {
		rank["textScore"] = bson.M{"$meta": "textScore"}
	}
	if raw := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(query)), "@"); usernamePrefixPattern.MatchString(raw) {
		rank["exact"] = bson.M{"$eq": bson.A{"$username", raw}}
		rank["prefix"] = bson.M{"$eq": bson.A{bson.M{"$indexOfBytes": bson.A{"$username", raw}}, 0}}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$addFields", Value: rank}},
		{{Key: "$facet", Value: bson.M{
			"rows": bson.A{
				bson.M{"$sort": bson.D{
					{Key: "exact", Value: -1},
					{Key: "prefix", Value: -1},
					{Key: "wordHits", Value: -1},
					{Key: "nameHits", Value: -1},
					{Key: "textScore", Value: -1},
					{Key: "followerCount", Value: -1},
					{Key: "_id", Value: 1},
				}},
				bson.M{"$skip": int64(page-1) * int64(limit)},
				bson.M{"$limit": int64(limit)},
				bson.M{"$project": bson.M{"search": 0}},
			},
			"total": bson.A{bson.M{"$count": "n"}},
		}}},
	}
	var rows []UserSearchDoc
	total, err := r.aggregatePage(ctx, r.usersCollection, pipeline, &rows)
	return rows, total, err
}

// aggregatePage runs a pipeline ending in a {rows, total} $facet
func (r *Repository) aggregatePage(ctx context.Context, coll *mongo.Collection, pipeline mongo.Pipeline, rows interface{}) (int64, error) {
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)
	var result []struct {
		Rows  bson.RawValue `bson:"rows"`
		Total []struct {
			N int64 `bson:"n"`
		} `bson:"total"`
	}
	if err := cursor.All(ctx, &result); err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return 0, nil
	}
	if err := result[0].Rows.Unmarshal(rows); err != nil {
		return 0, err
	}
	if len(result[0].Total) == 0 {
		return 0, nil
	}
	return result[0].Total[0].N, nil
}

// SearchTags returns tags that start with prefix (already normalised), most used first.
// Only public, non-deleted anchors count.
func (r *Repository) SearchTags(ctx context.Context, prefix string, limit int) ([]TagResult, error) {
	// Case-insensitive so tags saved before normalisation existed are still found
	startsWith := primitive.Regex{Pattern: "^" + regexp.QuoteMeta(prefix), Options: "i"}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"visibility": "public",
			"deletedAt":  nil,
			"tags":       startsWith,
		}}},
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$addFields", Value: bson.M{"lowerTag": bson.M{"$toLower": "$tags"}}}},
		{{Key: "$match", Value: bson.M{"lowerTag": startsWith}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$lowerTag",
			"count": bson.M{"$sum": 1},
		}}},
		// Equal counts are ordered by name, so suggestions don't jump around between keystrokes
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
		{{Key: "$limit", Value: limit}},
		{{Key: "$project", Value: bson.M{
			"_id":   0,
			"name":  "$_id",
			"count": 1,
		}}},
	}

	cursor, err := r.anchorsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []TagResult
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if results == nil {
		results = []TagResult{}
	}
	return results, nil
}
