package repositories

import (
	"regexp"
	"testing"
)

func matches(t *testing.T, query, title string) bool {
	t.Helper()
	rx := regexp.MustCompile("(?i)" + FuzzyPattern(NormalizeSearchText(query)))
	return rx.MatchString(title)
}

func TestFuzzyPatternMatchesSpellingVariants(t *testing.T) {
	cases := []struct {
		query, title string
		want         bool
	}{
		{"kasoskorlar", "Qasoskorlar: Final", true},
		{"qasoskorlar", "Qasoskorlar", true},
		{"ozbek", "O'zbek xalq ertaklari", true},
		{"o'zbek", "Oʻzbek xalq ertaklari", true},
		{"hayot", "Xayot mo'jizasi", true},
		{"misija", "Missija bajarildi", true},
		{"spider man", "Spider-Man: No Way Home", true},
		{"spiderman", "Spider-Man", false}, // no separator in query → needs the space
		{"titanik", "Интерстеллар", false},
		{"a.*b", "anything b", false}, // metacharacters are literal
	}
	for _, c := range cases {
		if got := matches(t, c.query, c.title); got != c.want {
			t.Errorf("query %q vs %q: got %v want %v (pattern %s)", c.query, c.title, got, c.want, FuzzyPattern(NormalizeSearchText(c.query)))
		}
	}
}

func TestTransliteration(t *testing.T) {
	if got := NormalizeSearchText("Қасоскорлар"); got != "qasoskorlar" {
		t.Errorf("cyr→lat: %q", got)
	}
	if got := ToCyrillic("qasoskorlar"); got != "қасоскорлар" {
		t.Errorf("lat→cyr: %q", got)
	}
	if got := ToCyrillic("o'zbek"); got != "ўзбек" {
		t.Errorf("lat→cyr digraph: %q", got)
	}
}

func TestSearchScoreOrdering(t *testing.T) {
	exact := SearchScore("titanik", "", "Titanik")
	prefix := SearchScore("titan", "", "Titanik")
	contains := SearchScore("tanik", "", "Titanik")
	fuzzy := SearchScore("kasos", "", "Qasoskorlar")
	code := SearchScore("A12", "A12", "Whatever")
	if !(code > exact && exact > prefix && prefix > contains && contains > fuzzy && fuzzy > 0) {
		t.Errorf("unexpected ordering: code=%d exact=%d prefix=%d contains=%d fuzzy=%d", code, exact, prefix, contains, fuzzy)
	}
}

func TestSearchWords(t *testing.T) {
	got := SearchWords("  Spider man: yo'l   yo'l uy ")
	want := []string{"spider", "man", "yo'l"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
