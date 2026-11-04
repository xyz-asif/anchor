// Package searchterms builds the prefix index used by search.
//
// MongoDB's $text search only matches whole (stemmed) words, ignores "stop words" such as
// "the", and treats a leading '-' as "exclude this word". That makes type-ahead impossible:
// "garde" never finds "Gardening". Instead, every searchable document stores the prefixes of
// its words (search.terms, indexed), and a query matches documents that contain every query
// word as a prefix of one of their words.
//
// Text is lowercased, accents on Latin letters are removed ("Café" matches "cafe"), and words
// are split on anything that isn't a letter, digit or combining mark ("web-design" is "web" +
// "design"). Documents and queries go through the same function, so they always agree.
package searchterms

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Version is stored with the terms; bump it when the algorithm changes and the backfill
// recomputes every document.
const Version = 1

const (
	// MaxPrefixLen: words are indexed up to this many characters; longer query words are cut
	// to the same length, so they still match
	MaxPrefixLen = 20
	// MaxQueryWords: words after this are ignored
	MaxQueryWords = 10
	// maxDescriptionWords: only the start of a long description is indexed
	maxDescriptionWords = 60
)

// Fields is the "search" sub-document stored on anchors and users
type Fields struct {
	Terms      []string `bson:"terms"`                // every prefix of every word (indexed)
	Title      []string `bson:"title,omitempty"`      // prefixes of the title/name words, for ranking
	TitleWords []string `bson:"titleWords,omitempty"` // whole title/name words, for ranking
	V          int      `bson:"v"`
}

// Words splits text into lowercase, accent-free words
func Words(s string) []string {
	s = fold(s)
	return strings.FieldsFunc(s, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.M, r))
	})
}

// fold lowercases and removes accents from Latin letters only; marks in other scripts
// (e.g. Devanagari vowel signs) are part of the word and stay
func fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevLatin := false
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) && prevLatin {
			continue
		}
		prevLatin = unicode.Is(unicode.Latin, r)
		b.WriteRune(unicode.ToLower(r))
	}
	return norm.NFC.String(b.String())
}

// prefixes returns the prefixes of word from 2 characters up to MaxPrefixLen (a 1-character
// word is its own only prefix)
func prefixes(word string) []string {
	runes := []rune(word)
	if len(runes) == 1 {
		return []string{word}
	}
	if len(runes) > MaxPrefixLen {
		runes = runes[:MaxPrefixLen]
	}
	out := make([]string, 0, len(runes)-1)
	for i := 2; i <= len(runes); i++ {
		out = append(out, string(runes[:i]))
	}
	return out
}

type set struct {
	seen map[string]bool
	list []string
}

func (s *set) add(items ...string) {
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	for _, it := range items {
		if !s.seen[it] {
			s.seen[it] = true
			s.list = append(s.list, it)
		}
	}
}

func (s *set) result() []string {
	if s.list == nil {
		return []string{}
	}
	return s.list
}

// ForAnchor builds the search fields for an anchor
func ForAnchor(title, description string, tagList []string) *Fields {
	var terms, titleTerms, titleWords set
	for _, w := range Words(title) {
		p := prefixes(w)
		terms.add(p...)
		titleTerms.add(p...)
		titleWords.add(w)
	}
	for _, t := range tagList {
		for _, w := range Words(t) {
			terms.add(prefixes(w)...)
		}
	}
	desc := Words(description)
	if len(desc) > maxDescriptionWords {
		desc = desc[:maxDescriptionWords]
	}
	for _, w := range desc {
		terms.add(prefixes(w)...)
	}
	return &Fields{Terms: terms.result(), Title: titleTerms.result(), TitleWords: titleWords.result(), V: Version}
}

// ForUser builds the search fields for a user: the username (whole, so "asif_sh" matches, and
// by part) and the display name
func ForUser(username, displayName string) *Fields {
	var terms, name, nameWords set
	if u := strings.ToLower(username); u != "" {
		terms.add(prefixes(u)...)
		name.add(prefixes(u)...)
		nameWords.add(u)
	}
	for _, w := range append(Words(username), Words(displayName)...) {
		p := prefixes(w)
		terms.add(p...)
		name.add(p...)
		nameWords.add(w)
	}
	return &Fields{Terms: terms.result(), Title: name.result(), TitleWords: nameWords.result(), V: Version}
}

// stopWords are dropped from a query that also has other words, so "the best pasta" finds
// "Pasta: the best" but also "Best pasta recipes". A query of only stop words keeps them.
var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true, "by": true,
	"for": true, "from": true, "how": true, "i": true, "in": true, "is": true, "it": true, "my": true,
	"of": true, "on": true, "or": true, "that": true, "the": true, "this": true, "to": true,
	"was": true, "what": true, "with": true, "your": true,
}

// QueryTerms turns what the user typed into the terms a document must contain. Every word is
// treated as the start of a word (so the half-typed last word matches), operators like '-' or
// quotes mean nothing, and stop words and single letters are dropped when other words are present.
func QueryTerms(q string) []string {
	words := Words(strings.TrimPrefix(strings.TrimSpace(q), "@"))
	var kept, all set
	for _, w := range words {
		n := utf8.RuneCountInString(w)
		if n > MaxPrefixLen {
			w = string([]rune(w)[:MaxPrefixLen])
		}
		all.add(w)
		// A single letter next to other words is usually a word still being typed
		// ("gardening t"), which would match almost nothing as a whole word
		if !stopWords[w] && n > 1 {
			kept.add(w)
		}
	}
	terms := kept.list
	if len(terms) == 0 {
		terms = all.list
	}
	if len(terms) > MaxQueryWords {
		terms = terms[:MaxQueryWords]
	}
	return terms
}
