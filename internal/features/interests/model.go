package interests

// Category represents an interest category/tag
type Category struct {
	Name  string `json:"name"`
	Slug  string `json:"slug"` // URL-friendly version
	Icon  string `json:"icon"` // Emoji icon
	Count int    `json:"count"`
	Score int    `json:"score"` // Weighted score based on source
}

// SuggestedInterestsResponse for GET /interests/suggested
type SuggestedInterestsResponse struct {
	Categories []Category `json:"categories"`
	BasedOn    string     `json:"basedOn"` // "personalized" or "popular"
}
