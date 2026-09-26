package repositories

import (
	"regexp"
	"strings"
	"unicode"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Search helpers shared by movie and series search.
//
// Goals:
//   - never pass raw user input to $regex (it was unescaped before);
//   - tolerate the spelling differences people actually type for Uzbek
//     titles: q/k, x/h, missing o'/g' apostrophes, doubled letters,
//     punctuation and extra spaces;
//   - match titles written in the other script (Cyrillic ↔ Latin).

var apostrophes = strings.NewReplacer("ʻ", "'", "ʼ", "'", "‘", "'", "’", "'", "`", "'", "´", "'")

var cyrToLat = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo", 'ж': "j", 'з': "z",
	'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r",
	'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "x", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sh",
	'ъ': "'", 'ы': "i", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya", 'ў': "o'", 'қ': "q", 'ғ': "g'", 'ҳ': "h",
}

// latToCyr is applied greedily, digraphs first.
var latToCyr = []struct{ lat, cyr string }{
	{"o'", "ў"}, {"g'", "ғ"}, {"sh", "ш"}, {"ch", "ч"}, {"yo", "ё"}, {"yu", "ю"}, {"ya", "я"}, {"ts", "ц"},
	{"a", "а"}, {"b", "б"}, {"d", "д"}, {"e", "е"}, {"f", "ф"}, {"g", "г"}, {"h", "ҳ"}, {"i", "и"},
	{"j", "ж"}, {"k", "к"}, {"l", "л"}, {"m", "м"}, {"n", "н"}, {"o", "о"}, {"p", "п"}, {"q", "қ"},
	{"r", "р"}, {"s", "с"}, {"t", "т"}, {"u", "у"}, {"v", "в"}, {"x", "х"}, {"y", "й"}, {"z", "з"},
}

// NormalizeSearchText lowercases, unifies apostrophes, transliterates
// Cyrillic to Latin and collapses whitespace.
func NormalizeSearchText(s string) string {
	s = apostrophes.Replace(strings.ToLower(strings.TrimSpace(s)))
	var b strings.Builder
	for _, r := range s {
		if lat, ok := cyrToLat[r]; ok {
			b.WriteString(lat)
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// ToCyrillic converts a normalized Latin query to (Uzbek) Cyrillic.
func ToCyrillic(latin string) string {
	var b strings.Builder
	for i := 0; i < len(latin); {
		matched := false
		for _, p := range latToCyr {
			if strings.HasPrefix(latin[i:], p.lat) {
				b.WriteString(p.cyr)
				i += len(p.lat)
				matched = true
				break
			}
		}
		if !matched {
			r := rune(latin[i])
			if r >= 0x80 { // keep multi-byte runes intact
				rr, size := decodeRune(latin[i:])
				b.WriteRune(rr)
				i += size
				continue
			}
			b.WriteByte(latin[i])
			i++
		}
	}
	return b.String()
}

func decodeRune(s string) (rune, int) {
	for i, r := range s {
		if i == 0 {
			return r, len(string(r))
		}
	}
	return 0, 1
}

const apostropheClass = `['ʻʼ‘’` + "`" + `´]`

// letter equivalence classes for common Uzbek spelling variants.
var fuzzyClasses = map[rune]string{
	'q': "[qkқк]", 'k': "[kqкқ]",
	'x': "[xhхҳ]", 'h': "[hxҳх]",
}

// FuzzyPattern builds a case-insensitive-safe regex (no user
// metacharacters) for a normalized Latin query.
func FuzzyPattern(q string) string {
	var b strings.Builder
	var prev rune
	for _, r := range q {
		switch {
		case r == '\'':
			b.WriteString(apostropheClass + "?")
		case unicode.IsSpace(r) || strings.ContainsRune("-–—:.,!?_/", r):
			if prev != ' ' {
				b.WriteString(`[\s\-–—:.,!?_/]*`)
			}
			r = ' '
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if r == prev {
				continue // doubled letter in the query; the '+' below covers it
			}
			if cls, ok := fuzzyClasses[r]; ok {
				b.WriteString(cls)
			} else {
				b.WriteString(regexp.QuoteMeta(string(r)))
			}
			b.WriteString("+")
			// o' / g' are often typed without the apostrophe.
			if r == 'o' || r == 'g' {
				b.WriteString(apostropheClass + "?")
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
		prev = r
	}
	return b.String()
}

// BuildTitleSearchFilter returns an $or filter matching the query against
// the given title fields in Latin (fuzzy) and Cyrillic forms, plus an exact
// code match and — for longer queries — a literal description match.
func BuildTitleSearchFilter(query string, titleFields []string, withDescription bool) bson.M {
	norm := NormalizeSearchText(query)
	if norm == "" {
		return bson.M{"_id": bson.M{"$exists": false}} // matches nothing
	}
	latRx := primitive.Regex{Pattern: FuzzyPattern(norm), Options: "i"}
	cyrRx := primitive.Regex{Pattern: regexp.QuoteMeta(ToCyrillic(norm)), Options: "i"}

	or := bson.A{}
	for _, f := range titleFields {
		or = append(or, bson.M{f: latRx}, bson.M{f: cyrRx})
	}
	if len(norm) <= 12 && !strings.Contains(norm, " ") {
		or = append(or, bson.M{"code": strings.ToUpper(strings.TrimSpace(query))})
	}
	if withDescription && len([]rune(norm)) >= 4 {
		or = append(or, bson.M{"description": primitive.Regex{Pattern: regexp.QuoteMeta(strings.TrimSpace(query)), Options: "i"}})
	}
	return bson.M{"$or": or}
}

// SearchWords returns the distinct words (≥3 letters) of a query, used as a
// fallback when the whole phrase finds nothing.
func SearchWords(query string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range strings.Fields(NormalizeSearchText(query)) {
		w = strings.Trim(w, "'-:.,!?")
		if len([]rune(w)) >= 3 && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// SearchScore ranks a hit: exact code/title > prefix > substring > fuzzy.
func SearchScore(query, code string, titles ...string) int {
	q := NormalizeSearchText(query)
	if q == "" {
		return 0
	}
	if code != "" && strings.EqualFold(strings.TrimSpace(query), code) {
		return 1000
	}
	best := 0
	fuzzy := regexp.MustCompile("(?i)" + FuzzyPattern(q))
	for _, t := range titles {
		n := NormalizeSearchText(t)
		if n == "" {
			continue
		}
		score := 0
		switch {
		case n == q:
			score = 900
		case strings.HasPrefix(n, q):
			score = 700
		case strings.Contains(n, " "+q):
			score = 600 // word start
		case strings.Contains(n, q):
			score = 500
		case fuzzy.MatchString(n):
			score = 300
		}
		if score > best {
			best = score
		}
	}
	return best
}
