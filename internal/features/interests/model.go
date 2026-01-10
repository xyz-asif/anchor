package interests

// Category represents an interest category
type Category struct {
	Name           string  `json:"name"`
	DisplayName    string  `json:"displayName"`
	AnchorCount    int     `json:"anchorCount"`
	RelevanceScore float64 `json:"relevanceScore"`
}

// BasedOn shows what the personalization is based on
type BasedOn struct {
	OwnAnchorTags      []string `json:"ownAnchorTags"`
	LikedAnchorTags    []string `json:"likedAnchorTags"`
	FollowedAnchorTags []string `json:"followedAnchorTags"`
}

// SuggestedInterestsQuery for GET /interests/suggested
type SuggestedInterestsQuery struct {
	Limit int `form:"limit,default=10" binding:"min=1,max=20"`
}

// SuggestedInterestsResponse for GET /interests/suggested
type SuggestedInterestsResponse struct {
	Categories   []Category `json:"categories"`
	Personalized bool       `json:"personalized"`
	BasedOn      *BasedOn   `json:"basedOn"`
}

// TagCountResult from aggregation
type TagCountResult struct {
	Name  string `bson:"name"`
	Count int    `bson:"count"`
}
