package feed

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// EncodeCursor creates a base64 encoded cursor string from timestamp and anchor ID
func EncodeCursor(timestamp time.Time, anchorID primitive.ObjectID) string {
	cursorData := FeedCursor{
		Timestamp: timestamp,
		AnchorID:  anchorID,
	}
	jsonBytes, _ := json.Marshal(cursorData)
	return base64.StdEncoding.EncodeToString(jsonBytes)
}

// DecodeCursor decodes a base64 encoded cursor string into a FeedCursor struct
func DecodeCursor(cursor string) (*FeedCursor, error) {
	if cursor == "" {
		return nil, nil
	}

	jsonBytes, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, errors.New("invalid cursor format: not base64")
	}

	var cursorData FeedCursor
	if err := json.Unmarshal(jsonBytes, &cursorData); err != nil {
		return nil, errors.New("invalid cursor format: invalid json")
	}

	if cursorData.Timestamp.IsZero() {
		return nil, errors.New("invalid cursor: missing timestamp")
	}
	if cursorData.AnchorID.IsZero() {
		return nil, errors.New("invalid cursor: missing anchor id")
	}

	return &cursorData, nil
}

// EncodeDiscoverCursor creates cursor for discovery feed
func EncodeDiscoverCursor(score *int, createdAt time.Time, anchorID primitive.ObjectID) string {
	cursorData := DiscoverCursor{
		Score:     score,
		CreatedAt: createdAt,
		AnchorID:  anchorID,
	}
	jsonBytes, _ := json.Marshal(cursorData)
	return base64.StdEncoding.EncodeToString(jsonBytes)
}

// DecodeDiscoverCursor decodes a discovery cursor
func DecodeDiscoverCursor(cursor string) (*DiscoverCursor, error) {
	if cursor == "" {
		return nil, nil
	}

	jsonBytes, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, errors.New("invalid cursor format")
	}

	var cursorData DiscoverCursor
	if err := json.Unmarshal(jsonBytes, &cursorData); err != nil {
		return nil, errors.New("invalid cursor data")
	}

	if cursorData.CreatedAt.IsZero() || cursorData.AnchorID.IsZero() {
		return nil, errors.New("invalid cursor: missing required fields")
	}

	return &cursorData, nil
}
