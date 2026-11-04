package feed

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// maxCursorLength bounds what we try to decode; real cursors are well under 200 characters
const maxCursorLength = 512

// errBadCursor is shown to the client for any cursor that can't be used
var errBadCursor = errors.New("This feed position is invalid or out of date. Please refresh the feed and try again")

// encodeCursorJSON uses URL-safe base64 without padding, so the cursor can be put in a query
// string as-is (standard base64 contains '+', which a query string turns into a space)
func encodeCursorJSON(v interface{}) string {
	jsonBytes, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(jsonBytes)
}

// decodeCursorJSON accepts the URL-safe form and, for clients holding older cursors, standard
// base64 (padded or not, including a '+' that arrived as a space)
func decodeCursorJSON(cursor string, v interface{}) error {
	if len(cursor) > maxCursorLength {
		return errBadCursor
	}
	raw := strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(cursor), " ", "+"), "=")
	jsonBytes, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		if jsonBytes, err = base64.RawStdEncoding.DecodeString(raw); err != nil {
			return errBadCursor
		}
	}
	if err := json.Unmarshal(jsonBytes, v); err != nil {
		return errBadCursor
	}
	return nil
}

// EncodeCursor creates a cursor string from timestamp and anchor ID
func EncodeCursor(timestamp time.Time, anchorID primitive.ObjectID) string {
	return encodeCursorJSON(FeedCursor{Timestamp: timestamp, AnchorID: anchorID})
}

// DecodeCursor decodes a home feed cursor; an empty cursor means the first page
func DecodeCursor(cursor string) (*FeedCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	var cursorData FeedCursor
	if err := decodeCursorJSON(cursor, &cursorData); err != nil {
		return nil, err
	}
	if cursorData.Timestamp.IsZero() || cursorData.AnchorID.IsZero() {
		return nil, errBadCursor
	}
	return &cursorData, nil
}

// EncodeDiscoverCursor creates cursor for discovery feed
func EncodeDiscoverCursor(score *int, createdAt time.Time, anchorID primitive.ObjectID) string {
	return encodeCursorJSON(DiscoverCursor{Score: score, CreatedAt: createdAt, AnchorID: anchorID})
}

// DecodeDiscoverCursor decodes a discovery cursor; an empty cursor means the first page
func DecodeDiscoverCursor(cursor string) (*DiscoverCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	var cursorData DiscoverCursor
	if err := decodeCursorJSON(cursor, &cursorData); err != nil {
		return nil, err
	}
	if cursorData.CreatedAt.IsZero() || cursorData.AnchorID.IsZero() {
		return nil, errBadCursor
	}
	return &cursorData, nil
}
