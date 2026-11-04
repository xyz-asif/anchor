package searchterms

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWordsFoldAccentsAndSplit(t *testing.T) {
	require.Equal(t, []string{"cafe", "creme", "brulee"}, Words("Café Crème-Brûlée!"))
	require.Equal(t, []string{"web", "design", "2026"}, Words("#Web_Design (2026)"))
	require.Equal(t, []string{"खाना", "पकाना"}, Words("खाना पकाना"), "marks in other scripts are kept")
}

func TestQueryTerms(t *testing.T) {
	require.Equal(t, []string{"garde"}, QueryTerms("Garde"))
	require.Equal(t, []string{"best", "pasta"}, QueryTerms("the best pasta"))
	require.Equal(t, []string{"the"}, QueryTerms("the"), "a query of only stop words keeps them")
	require.Equal(t, []string{"cats"}, QueryTerms(`-cats "`), "operators mean nothing")
	require.Equal(t, []string{"gardening"}, QueryTerms("gardening t"))
	require.Equal(t, []string{"asif", "sh"}, QueryTerms("@asif_sh"))
	require.Equal(t, []string{"cafe"}, QueryTerms("CAFÉ"))
}

func TestForAnchorAndUser(t *testing.T) {
	f := ForAnchor("Gardening Tips", "for small balconies", []string{"urban-farming"})
	for _, want := range []string{"ga", "garde", "gardening", "ti", "tips", "ba", "balconies", "urb", "farm"} {
		require.Contains(t, f.Terms, want)
	}
	require.Contains(t, f.Title, "garde")
	require.NotContains(t, f.Title, "balc")
	require.Equal(t, []string{"gardening", "tips"}, f.TitleWords)

	u := ForUser("asif_shaik", "Asif Shaik")
	for _, want := range []string{"as", "asif_", "asif_sh", "sha", "shaik"} {
		require.Contains(t, u.Terms, want)
	}
	long := ForAnchor("Supercalifragilisticexpialidocious", "", nil)
	require.Contains(t, long.Terms, "supercalifragilistic") // 20 characters
	require.Len(t, QueryTerms("supercalifragilisticexpialidocious")[0], 20)
}
