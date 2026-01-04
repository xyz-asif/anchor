package anchors

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// ValidateAnchorTitle validates the anchor title
func ValidateAnchorTitle(title string) error {
	title = strings.TrimSpace(title)
	if len(title) < 3 {
		return errors.New("title must be at least 3 characters long")
	}
	if len(title) > 100 {
		return errors.New("title cannot exceed 100 characters")
	}
	return nil
}

// ValidateDescription validates the anchor description
func ValidateDescription(description string) error {
	description = strings.TrimSpace(description)
	if len(description) > 500 {
		return errors.New("description cannot exceed 500 characters")
	}
	return nil
}

// ValidateTags validates the tags array
func ValidateTags(tags []string) error {
	if len(tags) > 5 {
		return errors.New("cannot have more than 5 tags")
	}

	// Regex for alphanumeric and hyphens only
	validTagPattern := regexp.MustCompile(`^[a-z0-9-]+$`)

	for i, tag := range tags {
		tag = strings.TrimSpace(tag)
		tag = strings.ToLower(tag)

		if len(tag) < 3 {
			return errors.New("each tag must be at least 3 characters long")
		}
		if len(tag) > 20 {
			return errors.New("each tag cannot exceed 20 characters")
		}
		if !validTagPattern.MatchString(tag) {
			return errors.New("tags can only contain lowercase letters, numbers, and hyphens")
		}

		tags[i] = tag
	}

	return nil
}

// ValidateVisibility validates the visibility value
func ValidateVisibility(visibility string) error {
	validVisibilities := map[string]bool{
		"private":  true,
		"unlisted": true,
		"public":   true,
	}

	if !validVisibilities[visibility] {
		return errors.New("visibility must be one of: private, unlisted, public")
	}

	return nil
}

// ValidateCoverMediaType validates the cover media type
func ValidateCoverMediaType(mediaType string) error {
	validTypes := map[string]bool{
		"icon":  true,
		"emoji": true,
		"image": true,
	}

	if !validTypes[mediaType] {
		return errors.New("cover media type must be one of: icon, emoji, image")
	}

	return nil
}

// NormalizeTags cleans and normalizes the tags array
func NormalizeTags(tags []string) []string {
	seen := make(map[string]bool)
	normalized := []string{}

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		tag = strings.ToLower(tag)

		// Skip empty tags and duplicates
		if tag == "" || seen[tag] {
			continue
		}

		seen[tag] = true
		normalized = append(normalized, tag)
	}

	return normalized
}

// ValidateItemType validates the item type
func ValidateItemType(itemType string) error {
	validTypes := map[string]bool{
		"url":   true,
		"image": true,
		"audio": true,
		"file":  true,
		"text":  true,
	}

	if !validTypes[itemType] {
		return errors.New("item type must be one of: url, image, audio, file, text")
	}

	return nil
}

// ValidateURL validates a URL format
func ValidateURL(urlStr string) error {
	urlStr = strings.TrimSpace(urlStr)

	if urlStr == "" {
		return errors.New("URL cannot be empty")
	}

	// Check if URL starts with http:// or https://
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return errors.New("URL must start with http:// or https://")
	}

	// Parse URL to validate format
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return errors.New("invalid URL format")
	}

	// Check if URL has a valid host
	if parsedURL.Host == "" {
		return errors.New("URL must have a valid host")
	}

	return nil
}

// ValidateTextContent validates text content
func ValidateTextContent(content string) error {
	content = strings.TrimSpace(content)

	if len(content) < 1 {
		return errors.New("text content cannot be empty")
	}

	if len(content) > 10000 {
		return errors.New("text content cannot exceed 10,000 characters")
	}

	return nil
}

// ValidateCreateAnchorRequest validates all fields in CreateAnchorRequest
func ValidateCreateAnchorRequest(req *CreateAnchorRequest) error {
	if err := ValidateAnchorTitle(req.Title); err != nil {
		return err
	}
	if err := ValidateDescription(req.Description); err != nil {
		return err
	}
	if len(req.Tags) > 0 {
		if err := ValidateTags(req.Tags); err != nil {
			return err
		}
	}
	if req.Visibility != nil {
		if err := ValidateVisibility(*req.Visibility); err != nil {
			return err
		}
	}
	if req.CoverMediaType != nil {
		if err := ValidateCoverMediaType(*req.CoverMediaType); err != nil {
			return err
		}
	}
	return nil
}

// ValidateUpdateAnchorRequest validates all non-nil fields in UpdateAnchorRequest
func ValidateUpdateAnchorRequest(req *UpdateAnchorRequest) error {
	if req.Title != nil {
		if err := ValidateAnchorTitle(*req.Title); err != nil {
			return err
		}
	}
	if req.Description != nil {
		if err := ValidateDescription(*req.Description); err != nil {
			return err
		}
	}
	if req.Tags != nil {
		if err := ValidateTags(req.Tags); err != nil {
			return err
		}
	}
	if req.Visibility != nil {
		if err := ValidateVisibility(*req.Visibility); err != nil {
			return err
		}
	}
	if req.CoverMediaType != nil {
		if err := ValidateCoverMediaType(*req.CoverMediaType); err != nil {
			return err
		}
	}
	return nil
}

// ValidateAddItemRequest validates the item request based on type
func ValidateAddItemRequest(req *AddItemRequest) error {
	if err := ValidateItemType(req.Type); err != nil {
		return err
	}

	switch req.Type {
	case ItemTypeURL:
		if req.URL == nil || *req.URL == "" {
			return errors.New("URL is required for URL type items")
		}
		if err := ValidateURL(*req.URL); err != nil {
			return err
		}
	case ItemTypeText:
		if req.Content == nil || *req.Content == "" {
			return errors.New("content is required for text type items")
		}
		if err := ValidateTextContent(*req.Content); err != nil {
			return err
		}
	}

	return nil
}
