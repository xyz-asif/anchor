package push

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Collection names (also used by account cleanup)
const (
	DevicesCollection  = "devices"
	SettingsCollection = "notification_settings"
)

type Repository struct {
	devices  *mongo.Collection
	settings *mongo.Collection
}

// sessionsCollection is the auth package's refresh-token sessions collection
const sessionsCollection = "refresh_tokens"

func NewRepository(db *mongo.Database) *Repository {
	devices := db.Collection(DevicesCollection)
	_, _ = devices.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "token", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "lastSeenAt", Value: -1}}},
		{Keys: bson.D{{Key: "sessionId", Value: 1}}, Options: options.Index().SetSparse(true)},
	})
	return &Repository{devices: devices, settings: db.Collection(SettingsCollection)}
}

// UpsertDevice stores a push token for the user. A token belongs to one app install, so if
// another account registered it before (e.g. account switch on the same phone), it moves
// to this user and the previous user stops getting pushes on that phone.
func (r *Repository) UpsertDevice(ctx context.Context, userID primitive.ObjectID, sessionID string, req RegisterDeviceRequest) (*Device, error) {
	now := time.Now()
	set := bson.M{"userId": userID, "platform": req.Platform, "appVersion": req.AppVersion, "lastSeenAt": now, "sessionId": sessionID}
	update := bson.M{"$set": set, "$setOnInsert": bson.M{"createdAt": now}}
	if sessionID == "" {
		// A token from before sessions existed: don't keep a session that no longer applies
		delete(set, "sessionId")
		update["$unset"] = bson.M{"sessionId": ""}
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var device Device
	err := r.devices.FindOneAndUpdate(ctx, bson.M{"token": req.Token}, update, opts).Decode(&device)
	if mongo.IsDuplicateKeyError(err) {
		// Two concurrent upserts of the same token: the other one inserted it, so update it
		delete(update, "$setOnInsert")
		err = r.devices.FindOneAndUpdate(ctx, bson.M{"token": req.Token}, update,
			options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&device)
	}
	if err != nil {
		return nil, err
	}
	r.trimDevices(ctx, userID)
	return &device, nil
}

// trimDevices keeps only the most recently seen devices of a user
func (r *Repository) trimDevices(ctx context.Context, userID primitive.ObjectID) {
	opts := options.Find().SetSort(bson.D{{Key: "lastSeenAt", Value: -1}, {Key: "_id", Value: -1}}).SetSkip(maxDevicesPerUser).
		SetProjection(bson.M{"_id": 1})
	cursor, err := r.devices.Find(ctx, bson.M{"userId": userID}, opts)
	if err != nil {
		return
	}
	var old []Device
	if cursor.All(ctx, &old) != nil || len(old) == 0 {
		return
	}
	ids := make([]primitive.ObjectID, len(old))
	for i, d := range old {
		ids[i] = d.ID
	}
	_, _ = r.devices.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
}

// DeleteDevice removes the user's token; it reports false if the user had no such token
func (r *Repository) DeleteDevice(ctx context.Context, userID primitive.ObjectID, token string) (bool, error) {
	res, err := r.devices.DeleteOne(ctx, bson.M{"token": token, "userId": userID})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// DeleteTokens removes tokens that FCM reported as no longer valid
func (r *Repository) DeleteTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := r.devices.DeleteMany(ctx, bson.M{"token": bson.M{"$in": tokens}})
	return err
}

// DevicesForUsers returns the devices of each user that should get pushes: devices whose login
// session has ended (logged out, revoked or expired) are left out, so a phone someone signed out
// of stops showing their notifications. Devices registered before sessions were recorded have
// no session and are kept.
func (r *Repository) DevicesForUsers(ctx context.Context, userIDs []primitive.ObjectID) (map[primitive.ObjectID][]Device, error) {
	out := make(map[primitive.ObjectID][]Device)
	if len(userIDs) == 0 {
		return out, nil
	}
	cursor, err := r.devices.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": bson.M{"$in": userIDs}}}},
		{{Key: "$lookup", Value: bson.M{
			"from": sessionsCollection,
			"let":  bson.M{"sid": "$sessionId"},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{"$expr": bson.M{"$eq": bson.A{"$tokenId", "$$sid"}}}},
				bson.M{"$match": bson.M{"revoked": bson.M{"$ne": true}, "expiresAt": bson.M{"$gt": time.Now()}}},
				bson.M{"$limit": 1},
				bson.M{"$project": bson.M{"_id": 1}},
			},
			"as": "session",
		}}},
		{{Key: "$match", Value: bson.M{"$or": bson.A{
			bson.M{"sessionId": bson.M{"$in": bson.A{nil, ""}}},
			bson.M{"session": bson.M{"$ne": bson.A{}}},
		}}}},
		{{Key: "$project", Value: bson.M{"session": 0}}},
	})
	if err != nil {
		return nil, err
	}
	var devices []Device
	if err := cursor.All(ctx, &devices); err != nil {
		return nil, err
	}
	for _, d := range devices {
		out[d.UserID] = append(out[d.UserID], d)
	}
	return out, nil
}

// GetSettings returns the user's push settings, or the defaults if never changed
func (r *Repository) GetSettings(ctx context.Context, userID primitive.ObjectID) (Settings, error) {
	var s Settings
	err := r.settings.FindOne(ctx, bson.M{"_id": userID}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return DefaultSettings(userID), nil
	}
	return s, err
}

// SettingsForUsers returns settings for each user (defaults for users without saved settings)
func (r *Repository) SettingsForUsers(ctx context.Context, userIDs []primitive.ObjectID) (map[primitive.ObjectID]Settings, error) {
	out := make(map[primitive.ObjectID]Settings, len(userIDs))
	for _, id := range userIDs {
		out[id] = DefaultSettings(id)
	}
	if len(userIDs) == 0 {
		return out, nil
	}
	cursor, err := r.settings.Find(ctx, bson.M{"_id": bson.M{"$in": userIDs}})
	if err != nil {
		return nil, err
	}
	var saved []Settings
	if err := cursor.All(ctx, &saved); err != nil {
		return nil, err
	}
	for _, s := range saved {
		out[s.UserID] = s
	}
	return out, nil
}

// SaveSettings stores the user's push settings
func (r *Repository) SaveSettings(ctx context.Context, s Settings) error {
	s.UpdatedAt = time.Now()
	_, err := r.settings.ReplaceOne(ctx, bson.M{"_id": s.UserID}, s, options.Replace().SetUpsert(true))
	return err
}

// UpdateSettings changes only the provided switches in one atomic update and returns the
// result. (Reading, changing and saving the whole document let two quick toggles overwrite
// each other.) Switches never saved before start from the defaults (on).
func (r *Repository) UpdateSettings(ctx context.Context, userID primitive.ObjectID, req UpdateSettingsRequest) (Settings, error) {
	set := bson.M{"updatedAt": time.Now()}
	onInsert := bson.M{}
	defaults := DefaultSettings(userID)
	defaultFields := UpdateSettingsRequest{
		Enabled: &defaults.Enabled, Likes: &defaults.Likes, Comments: &defaults.Comments, Replies: &defaults.Replies,
		Mentions: &defaults.Mentions, Follows: &defaults.Follows, Clones: &defaults.Clones, AnchorUpdates: &defaults.AnchorUpdates,
	}.fields()
	for field, v := range req.fields() {
		if v != nil {
			set[field] = *v
		} else {
			onInsert[field] = *defaultFields[field]
		}
	}
	update := bson.M{"$set": set}
	if len(onInsert) > 0 {
		update["$setOnInsert"] = onInsert
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var s Settings
	err := r.settings.FindOneAndUpdate(ctx, bson.M{"_id": userID}, update, opts).Decode(&s)
	if mongo.IsDuplicateKeyError(err) {
		// Two first-time saves raced: the document exists now, so update it
		delete(update, "$setOnInsert")
		err = r.settings.FindOneAndUpdate(ctx, bson.M{"_id": userID}, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&s)
	}
	return s, err
}
