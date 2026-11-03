package tags

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"#Travel":       "travel",
		"  Web Design ": "web-design",
		"web_design":    "web-design",
		"AI":            "ai",
		"x":             "",
		"#":             "",
		"खाना":          "खाना",
		"--go--":        "go",
		"c++":           "", // "c" is shorter than MinLen
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := NormalizeList([]string{"#Books", "books", "BOOKS", "x"}); len(got) != 1 || got[0] != "books" {
		t.Errorf("NormalizeList dedupe failed: %v", got)
	}
}
