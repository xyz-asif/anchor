package anchors

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/xyz-asif/gotodo/internal/pkg/tags"
)

// ValidateAnchorTitle validates the anchor title
// (spaces at either end don't count, so "   ab" is too short)
func ValidateAnchorTitle(title string) error {
	n := utf8.RuneCountInString(strings.TrimSpace(title))
	if n < 3 {
		return errors.New("The title needs at least 3 characters")
	}
	if n > 100 {
		return fmt.Errorf("The title can be at most 100 characters (this one has %d)", n)
	}
	return nil
}

// ValidateDescription validates the anchor description
func ValidateDescription(description string) error {
	if n := utf8.RuneCountInString(strings.TrimSpace(description)); n > 500 {
		return fmt.Errorf("The description can be at most 500 characters (this one has %d)", n)
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

// maxURLLength is the longest link accepted (browsers and most servers cap URLs around 2K)
const maxURLLength = 2048

// ValidateURL validates a URL format
func ValidateURL(urlStr string) error {
	urlStr = strings.TrimSpace(urlStr)

	if urlStr == "" {
		return errors.New("Please enter a link")
	}
	if len(urlStr) > maxURLLength {
		return fmt.Errorf("This link is too long (the maximum is %d characters)", maxURLLength)
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return errors.New("This doesn't look like a valid link. Please check it and try again")
	}
	// Scheme check on the parsed URL, so "HTTPS://..." works and "javascript:" never does
	if scheme := strings.ToLower(parsedURL.Scheme); scheme != "http" && scheme != "https" {
		return errors.New("Links must start with http:// or https://")
	}
	if parsedURL.Hostname() == "" {
		return errors.New("This link is missing a website address (for example https://example.com)")
	}

	return nil
}

// maxTextContent is the longest text item, in characters (not bytes, so every language gets the same room)
const maxTextContent = 10000

// ValidateTextContent validates text content
func ValidateTextContent(content string) error {
	content = strings.TrimSpace(content)

	if content == "" {
		return errors.New("A text item can't be empty")
	}
	if n := utf8.RuneCountInString(content); n > maxTextContent {
		return fmt.Errorf("Text items can be at most %d characters long (this one has %d)", maxTextContent, n)
	}

	return nil
}

// maxTags is the most tags an anchor can have
const maxTags = 5

// NormalizeAndValidateTags returns the canonical tags (same rules as interests and tag feeds),
// rejecting tags that would otherwise be silently dropped and lists that are too long.
func NormalizeAndValidateTags(raw []string) ([]string, error) {
	for _, t := range raw {
		if strings.TrimSpace(t) != "" && tags.Normalize(t) == "" {
			return nil, fmt.Errorf(`"%s" can't be used as a tag: tags need %d to %d letters or numbers (hyphens allowed)`,
				t, tags.MinLen, tags.MaxLen)
		}
	}
	out := tags.NormalizeList(raw)
	if len(out) > maxTags {
		return nil, fmt.Errorf("An anchor can have at most %d tags (you sent %d)", maxTags, len(out))
	}
	return out, nil
}

// ValidateCover checks that a cover's value fits its type: a short icon name, an emoji, or an
// http(s) image link. Both empty clears the cover.
func ValidateCover(coverType, value string) error {
	if coverType == "" && value == "" {
		return nil
	}
	if strings.ContainsAny(value, "\x00\n\r\t") {
		return errors.New("The cover can't contain line breaks or control characters")
	}
	switch coverType {
	case "":
		return errors.New(`Please also send "coverMediaType" (icon, emoji or image) with the cover`)
	case "icon":
		if n := utf8.RuneCountInString(value); n == 0 || n > 50 {
			return errors.New("An icon cover needs an icon name of 1 to 50 characters")
		}
	case "emoji":
		if n := utf8.RuneCountInString(value); n == 0 || n > 16 {
			return errors.New("An emoji cover needs a single emoji")
		}
	case "image":
		if value == "" {
			return errors.New("An image cover needs the image link in \"coverMediaValue\"")
		}
		if err := ValidateURL(value); err != nil {
			return fmt.Errorf("The cover image link isn't valid: %s", lowerFirst(err.Error()))
		}
	default:
		return errors.New("The cover type must be icon, emoji or image")
	}
	return nil
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return strings.ToLower(string(r)) + s[size:]
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
			return errors.New(`A link item needs a "url"`)
		}
		if err := ValidateURL(*req.URL); err != nil {
			return err
		}
	case ItemTypeText:
		if req.Content == nil || *req.Content == "" {
			return errors.New(`A text item needs "content"`)
		}
		if err := ValidateTextContent(*req.Content); err != nil {
			return err
		}
	}

	return nil
}

// ValidateReorderItemsRequest validates the reorder request
func ValidateReorderItemsRequest(req *ReorderItemsRequest, expectedCount int) error {
	if len(req.ItemIDs) == 0 {
		return errors.New(`Please send the new order as "itemIds": a list of every item ID in this anchor`)
	}

	if len(req.ItemIDs) != expectedCount {
		return fmt.Errorf("The new order must list all %d items of this anchor exactly once (you sent %d)", expectedCount, len(req.ItemIDs))
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for _, id := range req.ItemIDs {
		if seen[id] {
			return fmt.Errorf(`Item "%s" appears more than once in the new order`, id)
		}
		seen[id] = true
	}

	return nil
}
