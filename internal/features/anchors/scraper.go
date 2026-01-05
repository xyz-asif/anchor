package anchors

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// FetchURLMetadata fetches and parses metadata from a given URL
func FetchURLMetadata(ctx context.Context, targetURL string) (*URLData, error) {
	// Create client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}

	// Set User-Agent to avoid blocks
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AnchorBot/1.0)")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &URLData{OriginalURL: targetURL}, nil
	}

	return parseHTMLMetadata(resp.Body, targetURL), nil
}

// parseHTMLMetadata parses the HTML body and extracts metadata
func parseHTMLMetadata(body io.Reader, baseURL string) *URLData {
	data := &URLData{
		OriginalURL: baseURL,
	}

	doc, err := html.Parse(body)
	if err != nil {
		return data
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if n.FirstChild != nil {
					data.Title = n.FirstChild.Data
				}
			case "meta":
				name := getAttr(n, "name")
				property := getAttr(n, "property")
				content := getAttr(n, "content")

				switch {
				case property == "og:title":
					if data.Title == "" { // Only override if empty (or maybe prefer OG?) Spec doesn't specify priority for title. Usually OG is better.
						data.Title = content
					}
				case name == "description" && data.Description == "":
					data.Description = content
				case property == "og:description": // Priority over meta description
					data.Description = content // Overwrite if exists, as it's higher priority
				case property == "og:image": // Priority over twitter:image
					data.Thumbnail = resolveURL(baseURL, content)
				case name == "twitter:image" && data.Thumbnail == "":
					data.Thumbnail = resolveURL(baseURL, content)
				}
			case "link":
				rel := getAttr(n, "rel")
				href := getAttr(n, "href")

				if strings.Contains(rel, "icon") && href != "" {
					data.Favicon = resolveURL(baseURL, href)
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	return data
}

// resolveURL resolves relative URLs against the base URL
func resolveURL(baseURLStr, relativeURL string) string {
	if relativeURL == "" {
		return ""
	}

	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		return relativeURL
	}

	relURL, err := url.Parse(relativeURL)
	if err != nil {
		return relativeURL // Return as is if parsing fails
	}

	return baseURL.ResolveReference(relURL).String()
}

// getAttr extracts the value of a specific attribute from an HTML node
func getAttr(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}
