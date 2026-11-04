package anchors

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"
)

// maxPreviewBodyBytes caps how much of a remote page is read when scraping metadata
const maxPreviewBodyBytes = 2 << 20 // 2MB

// errBlockedAddress is returned when a URL resolves to a non-public address
var errBlockedAddress = errors.New("destination address is not allowed")

// blockedNets are special-purpose ranges not covered by the net.IP helpers below,
// including IPv6 translation prefixes that can wrap an internal IPv4 address.
var blockedNets = func() []*net.IPNet {
	var nets []*net.IPNet
	for _, cidr := range []string{
		"0.0.0.0/8",      // "this network"
		"100.64.0.0/10",  // carrier-grade NAT (also Alibaba Cloud metadata 100.100.100.200)
		"192.0.0.0/24",   // IETF protocol assignments
		"198.18.0.0/15",  // benchmarking
		"240.0.0.0/4",    // reserved
		"64:ff9b::/96",   // NAT64 (wraps IPv4)
		"64:ff9b:1::/48", // local-use NAT64
		"2002::/16",      // 6to4 (wraps IPv4)
		"2001::/32",      // Teredo
		"fc00::/7",       // unique local
	} {
		_, n, _ := net.ParseCIDR(cidr)
		nets = append(nets, n)
	}
	return nets
}()

// isPublicIP reports whether ip is a globally routable unicast address
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// safeHTTPClient returns a client that refuses to connect to private, loopback,
// link-local (incl. cloud metadata) or otherwise non-public addresses. The check
// runs at dial time, so it also covers redirects and DNS rebinding.
func safeHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || !isPublicIP(ip) {
				return errBlockedAddress
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("unsupported redirect scheme")
			}
			return nil
		},
	}
}

// FetchURLMetadata fetches and parses metadata from a given URL.
// Only http(s) URLs pointing at public addresses are fetched.
func FetchURLMetadata(ctx context.Context, targetURL string) (*URLData, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("only absolute http(s) URLs are supported")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}

	// Set User-Agent to avoid blocks
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AnchorBot/1.0)")

	resp, err := safeHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &URLData{OriginalURL: targetURL}, nil
	}

	// Relative links on the page are relative to where redirects ended up, not the URL we asked for
	baseURL := targetURL
	if resp.Request != nil && resp.Request.URL != nil {
		baseURL = resp.Request.URL.String()
	}
	data := parseHTMLMetadata(io.LimitReader(resp.Body, maxPreviewBodyBytes), baseURL)
	data.OriginalURL = targetURL
	return data, nil
}

// Limits on scraped text, so a hostile page can't store megabytes in an item
const (
	maxScrapedTitle       = 300
	maxScrapedDescription = 1000
)

// truncateRunes cuts s to at most n characters
func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n])) + "…"
	}
	return s
}

// safeLink keeps only http(s) links of sane length. Pages can declare og:image or icons as
// "javascript:" or "data:" URLs, which clients would otherwise render as-is.
func safeLink(link string) string {
	if link == "" || len(link) > maxURLLength {
		return ""
	}
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return link
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

	data.Title = truncateRunes(data.Title, maxScrapedTitle)
	data.Description = truncateRunes(data.Description, maxScrapedDescription)
	data.Thumbnail = safeLink(data.Thumbnail)
	data.Favicon = safeLink(data.Favicon)
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
