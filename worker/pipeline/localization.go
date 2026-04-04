package pipeline

import (
	"strings"
)

// UzbekTitleMap maps known English movie titles to their Uzbek names
// These are the natural Uzbek names used in Uzbekistan
var UzbekTitleMap = map[string]string{
	// Fast & Furious series
	"The Fast and the Furious":              "Forsaj",
	"2 Fast 2 Furious":                      "Forsaj 2",
	"The Fast and the Furious: Tokyo Drift": "Forsaj: Tokio Drift",
	"Fast & Furious":                        "Forsaj 4",
	"Fast Five":                             "Forsaj 5",
	"Furious 7":                             "Forsaj 7",
	"The Fate of the Furious":               "Forsaj 8",
	"F9: The Fast Saga":                     "Forsaj 9",
	"Fast X":                                "Forsaj 10",

	// Marvel/C superhero movies
	"Iron Man":                "Temir Odam",
	"Thor":                    "Tor",
	"Captain America":         "Kapitan Amerika",
	"Spider-Man":              "Odam-oyana",
	"Avengers":                "Qasoskorlar",
	"Guardians of the Galaxy": "Galaktika Qo'riqchilari",

	// Other popular titles
	"Taken":                    "Olib qo'yilgan",
	"John Wick":                "Jon Vik",
	"Die Hard":                 "Qattiq O'lim",
	"Rambo":                    "Rambo",
	"Rocky":                    "Rokki",
	"Terminator":               "Terminator",
	"Transformers":             "Transformers",
	"Avatar":                   "Avatar",
	"Jurassic Park":            "Yurassic Park",
	"Pirates of the Caribbean": "Karib Dengizlari Qaroqchilari",
	"Harry Potter":             "Gari Potter",
	"Star Wars":                "Yulduzlar Urushi",
	"James Bond":               "Jeyms Bond",
	"Mission: Impossible":      "Mumkin Emas Missiya",

	// Common translations
	"The Shawshank Redemption": "Shoushenk Amali",
	"The Godfather":            "Ota",
	"The Dark Knight":          "Qorong'i Ritsar",
	"Forrest Gump":             "Forrest Gump",
	"The Matrix":               "Matritsa",
	"Inception":                "Boshlash",
	"Interstellar":             "Yulduzlararo",
	"The Lord of the Rings":    "Uzuklar Egasi",
}

// UzbekGenreMap maps English genre names to Uzbek
var UzbekGenreMap = map[string]string{
	"Action":          "Jangovar",
	"Adventure":       "Sarguzasht",
	"Animation":       "Animatsiya",
	"Comedy":          "Komediya",
	"Crime":           "Jinoyat",
	"Documentary":     "Hujjatli film",
	"Drama":           "Drama",
	"Family":          "Oila",
	"Fantasy":         "Fantastika",
	"History":         "Tarixiy",
	"Horror":          "Qorong'i",
	"Kids":            "Bolalar",
	"Music":           "Musiqa",
	"Mystery":         "Sir",
	"News":            "Yangiliklar",
	"Romance":         "Romantika",
	"Science Fiction": "Ilmiy fantastika",
	"Sci-Fi":          "Ilmiy fantastika",
	"Soap":            "Serial",
	"Talk":            "Talk-shou",
	"Thriller":        "Triller",
	"TV Movie":        "TV film",
	"War":             "Urush",
	"Western":         "G'arbiy",
}

// UzbekCountryMap maps English country names to Uzbek
var UzbekCountryMap = map[string]string{
	"Afghanistan":              "Afg'oniston",
	"Albania":                  "Albaniya",
	"Algeria":                  "Jazoir",
	"Argentina":                "Argentina",
	"Armenia":                  "Armaniston",
	"Australia":                "Avstraliya",
	"Austria":                  "Avstriya",
	"Azerbaijan":               "O'zbekiston",
	"Bahamas":                  "Bagama orollari",
	"Bahrain":                  "Bahrayn",
	"Bangladesh":               "Bangladesh",
	"Belgium":                  "Belgiya",
	"Bolivia":                  "Boliviya",
	"Bosnia and Herzegovina":   "Bosniya va Gertsogovina",
	"Brazil":                   "Braziliya",
	"Bulgaria":                 "Bolgariya",
	"Cambodia":                 "Kambodja",
	"Cameroon":                 "Kamerun",
	"Canada":                   "Kanada",
	"Chile":                    "Chili",
	"China":                    "Xitoy",
	"Colombia":                 "Kolumbiya",
	"Costa Rica":               "Kosta Rika",
	"Croatia":                  "Xorvatiya",
	"Cuba":                     "Kuba",
	"Czech Republic":           "Chexiya",
	"Czechia":                  "Chexiya",
	"Denmark":                  "Daniya",
	"Dominican Republic":       "Dominikan Respublikasi",
	"Ecuador":                  "Ekvador",
	"Egypt":                    "Misr",
	"El Salvador":              "Salvador",
	"Estonia":                  "Estoniya",
	"Finland":                  "Finlandiya",
	"France":                   "Fransiya",
	"Georgia":                  "Gruziya",
	"Germany":                  "Germaniya",
	"Ghana":                    "Gana",
	"Greece":                   "Gretsiya",
	"Guatemala":                "Gvatemala",
	"Haiti":                    "Gaiti",
	"Honduras":                 "Gonduras",
	"Hong Kong":                "Gonkong",
	"Hungary":                  "Vengriya",
	"Iceland":                  "Islandiya",
	"India":                    "Hindiston",
	"Indonesia":                "Indoneziya",
	"Iran":                     "Eron",
	"Iraq":                     "Iroq",
	"Ireland":                  "Irlandiya",
	"Israel":                   "Isroil",
	"Italy":                    "Italiya",
	"Jamaica":                  "Yamayka",
	"Japan":                    "Yaponiya",
	"Jordan":                   "Iordaniya",
	"Kazakhstan":               "Qozog'iston",
	"Kenya":                    "Keniya",
	"North Korea":              "Shimoliy Koreya",
	"South Korea":              "Janubiy Koreya",
	"Kuwait":                   "Quvayt",
	"Latvia":                   "Latviya",
	"Lebanon":                  "Livan",
	"Libya":                    "Liviya",
	"Lithuania":                "Litva",
	"Luxembourg":               "Lyuksemburg",
	"Malaysia":                 "Malayziya",
	"Malta":                    "Malta",
	"Mexico":                   "Meksika",
	"Moldova":                  "Moldova",
	"Monaco":                   "Monako",
	"Mongolia":                 "Mongoliya",
	"Morocco":                  "Marokash",
	"Nepal":                    "Nepal",
	"Netherlands":              "Niderlandiya",
	"New Zealand":              "Yangi Zelandiya",
	"Nicaragua":                "Nikaragua",
	"Nigeria":                  "Nigeriya",
	"Norway":                   "Norvegiya",
	"Oman":                     "Ummon",
	"Pakistan":                 "Pokiston",
	"Palestine":                "Falastin",
	"Panama":                   "Panama",
	"Paraguay":                 "Paragvay",
	"Peru":                     "Peru",
	"Philippines":              "Filippin",
	"Poland":                   "Polsha",
	"Portugal":                 "Portugaliya",
	"Puerto Rico":              "Puerto-Riko",
	"Qatar":                    "Qatar",
	"Romania":                  "Ruminiya",
	"Russia":                   "Rossiya",
	"Russian Federation":       "Rossiya",
	"Saudi Arabia":             "Saudiya Arabistoni",
	"Senegal":                  "Senegal",
	"Serbia":                   "Serbiya",
	"Singapore":                "Singapur",
	"Slovakia":                 "Slovakiya",
	"Slovenia":                 "Sloveniya",
	"South Africa":             "Janubiy Afrika",
	"Spain":                    "Ispaniya",
	"Sri Lanka":                "Shri Lanka",
	"Sweden":                   "Shvetsiya",
	"Switzerland":              "Shveytsariya",
	"Syria":                    "Suriya",
	"Taiwan":                   "Tayvan",
	"Thailand":                 "Tailand",
	"Tunisia":                  "Tunis",
	"Turkey":                   "Turkiya",
	"Ukraine":                  "Ukraina",
	"United Arab Emirates":     "Birlashgan Arab Amirliklari",
	"United Kingdom":           "Buyuk Britaniya",
	"United States of America": "Amerika Qo'shma Shtatlari",
	"United States":            "Amerika Qo'shma Shtatlari",
	"USA":                      "Amerika Qo'shma Shtatlari",
	"Uruguay":                  "Urugvay",
	"Venezuela":                "Venesuela",
	"Vietnam":                  "Vyetnam",
	"Yemen":                    "Yaman",
	"Zimbabwe":                 "Zimbabve",
}

// LocalizedMetadata holds original and localized display metadata
type LocalizedMetadata struct {
	// Original TMDB metadata
	OriginalTitle     string
	OriginalDesc      string
	OriginalGenres    []string
	OriginalCountries []string

	// Uzbek display metadata
	TitleUz       string
	DescriptionUz string
	GenresUz      []string
	CountriesUz   []string

	// Whether TMDB had Uzbek translations
	HasUzbekTMDB bool
}

// LocalizeMetadata converts English metadata to Uzbek display format
// Note: TitleUz and DescriptionUz are left empty - they should be populated from TMDB Uzbek translations
// Genres and countries are localized using our mapping
func LocalizeMetadata(originalTitle, originalDesc string, genres, countries []string) *LocalizedMetadata {
	result := &LocalizedMetadata{
		OriginalTitle:     originalTitle,
		OriginalDesc:      originalDesc,
		OriginalGenres:    genres,
		OriginalCountries: countries,
		HasUzbekTMDB:      false,
	}

	// Localize genres and countries using our mapping
	result.GenresUz = LocalizeGenres(genres)
	result.CountriesUz = LocalizeCountries(countries)

	// TitleUz and DescriptionUz are left empty - they should be populated from TMDB Uzbek translations
	// If no TMDB translation is available, frontend will fall back to original fields

	return result
}

// LocalizeTitle converts an English movie title to Uzbek
// Returns the natural Uzbek name if available, otherwise returns empty string
// to indicate that the caller should use TMDB Uzbek translation or original title
func LocalizeTitle(originalTitle string) string {
	if originalTitle == "" {
		return ""
	}

	// First check exact match in our map
	if uzTitle, ok := UzbekTitleMap[originalTitle]; ok {
		return uzTitle
	}

	// Try case-insensitive match
	lowerTitle := strings.ToLower(originalTitle)
	for eng, uz := range UzbekTitleMap {
		if strings.ToLower(eng) == lowerTitle {
			return uz
		}
	}

	// No match found - return empty to indicate fallback needed
	return ""
}

// LocalizeGenres converts a slice of English genre names to Uzbek
func LocalizeGenres(genres []string) []string {
	if len(genres) == 0 {
		return []string{}
	}

	uzbekGenres := make([]string, 0, len(genres))
	for _, genre := range genres {
		if uz, ok := UzbekGenreMap[genre]; ok {
			uzbekGenres = append(uzbekGenres, uz)
		} else {
			// Keep original if no mapping found
			uzbekGenres = append(uzbekGenres, genre)
		}
	}
	return uzbekGenres
}

// LocalizeCountries converts a slice of English country names to Uzbek
func LocalizeCountries(countries []string) []string {
	if len(countries) == 0 {
		return []string{}
	}

	uzbekCountries := make([]string, 0, len(countries))
	for _, country := range countries {
		if uz, ok := UzbekCountryMap[country]; ok {
			uzbekCountries = append(uzbekCountries, uz)
		} else {
			// Keep original if no mapping found
			uzbekCountries = append(uzbekCountries, country)
		}
	}
	return uzbekCountries
}

// LocalizeSingleCountry converts a single country name to Uzbek
func LocalizeSingleCountry(country string) string {
	if country == "" {
		return ""
	}
	if uz, ok := UzbekCountryMap[country]; ok {
		return uz
	}
	return country
}

// LocalizeSingleGenre converts a single genre name to Uzbek
func LocalizeSingleGenre(genre string) string {
	if genre == "" {
		return ""
	}
	if uz, ok := UzbekGenreMap[genre]; ok {
		return uz
	}
	return genre
}

// GetDisplayTitle returns the Uzbek title if available, falls back to original
func GetDisplayTitle(titleUz, originalTitle string) string {
	if titleUz != "" {
		return titleUz
	}
	return originalTitle
}

// GetDisplayDescription returns the Uzbek description if available, falls back to original
func GetDisplayDescription(descUz, originalDesc string) string {
	if descUz != "" {
		return descUz
	}
	return originalDesc
}

// GetDisplayGenres returns Uzbek genres, falls back to original
func GetDisplayGenres(genresUz, genresOriginal []string) []string {
	if len(genresUz) > 0 {
		return genresUz
	}
	return genresOriginal
}

// GetDisplayCountries returns Uzbek countries, falls back to original
func GetDisplayCountries(countriesUz, countriesOriginal []string) []string {
	if len(countriesUz) > 0 {
		return countriesUz
	}
	return countriesOriginal
}

// NormalizeCountryCode normalizes ISO country codes to country names
func NormalizeCountryCode(code string) string {
	// Common ISO 3166-1 alpha-2 to country name mapping
	codeToCountry := map[string]string{
		"US": "United States of America",
		"GB": "United Kingdom",
		"DE": "Germany",
		"FR": "France",
		"ES": "Spain",
		"IT": "Italy",
		"RU": "Russia",
		"CN": "China",
		"JP": "Japan",
		"KR": "South Korea",
		"IN": "India",
		"BR": "Brazil",
		"CA": "Canada",
		"AU": "Australia",
		"MX": "Mexico",
		"TR": "Turkey",
		"SA": "Saudi Arabia",
		"AE": "United Arab Emirates",
		"NL": "Netherlands",
		"SE": "Sweden",
		"NO": "Norway",
		"DK": "Denmark",
		"FI": "Finland",
		"PL": "Poland",
		"CZ": "Czech Republic",
		"HU": "Hungary",
		"GR": "Greece",
		"PT": "Portugal",
		"BE": "Belgium",
		"AT": "Austria",
		"CH": "Switzerland",
		"IE": "Ireland",
		"NZ": "New Zealand",
		"ZA": "South Africa",
		"AR": "Argentina",
		"CL": "Chile",
		"CO": "Colombia",
		"PE": "Peru",
		"VE": "Venezuela",
		"TH": "Thailand",
		"VN": "Vietnam",
		"ID": "Indonesia",
		"MY": "Malaysia",
		"SG": "Singapore",
		"PH": "Philippines",
		"PK": "Pakistan",
		"EG": "Egypt",
		"NG": "Nigeria",
		"KE": "Kenya",
		"IL": "Israel",
		"UA": "Ukraine",
		"KZ": "Kazakhstan",
		"IR": "Iran",
		"IQ": "Iraq",
		"IS": "Iceland",
		"RO": "Romania",
		"HR": "Croatia",
		"RS": "Serbia",
		"BG": "Bulgaria",
		"LT": "Lithuania",
		"LV": "Latvia",
		"EE": "Estonia",
		"SK": "Slovakia",
		"SI": "Slovenia",
	}

	if name, ok := codeToCountry[strings.ToUpper(code)]; ok {
		return name
	}
	return code
}
