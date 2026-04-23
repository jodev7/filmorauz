// Uzbek genre mappings.
// Keys are English genre names; lookup is case-insensitive (see UZBEK_GENRE_LOOKUP).
// Backend stores genres as lowercase English; display always uses Uzbek labels below.
export const UZBEK_GENRE_MAP: Record<string, string> = {
	"Action": "Jangari",
	"Adventure": "Sarguzasht",
	"Animation": "Animatsiya",
	"Comedy": "Komediya",
	"Crime": "Jinoyat",
	"Documentary": "Hujjatli",
	"Drama": "Drama",
	"Family": "Oila",
	"Fantasy": "Fantastika",
	"History": "Tarixiy",
	"Horror": "Dahshat",
	"Kids": "Bolalar",
	"Music": "Musiqa",
	"Mystery": "Sirli",
	"News": "Yangiliklar",
	"Romance": "Romantika",
	"Science Fiction": "Ilmiy fantastika",
	"Sci-Fi": "Ilmiy fantastika",
	"Soap": "Serial",
	"Talk": "Talk-shou",
	"Thriller": "Triller",
	"TV Movie": "TV film",
	"War": "Urush",
	"Western": "Vestern",
	"Cartoon": "Multfilm",
};

// Case-insensitive lookup — DB stores lowercase, legacy data may be capitalized.
const UZBEK_GENRE_LOOKUP: Record<string, string> = Object.fromEntries(
	Object.entries(UZBEK_GENRE_MAP).map(([k, v]) => [k.toLowerCase(), v])
);

// Uzbek country mappings
export const UZBEK_COUNTRY_MAP: Record<string, string> = {
	"Afghanistan": "Afg'oniston",
	"Albania": "Albaniya",
	"Algeria": "Jazoir",
	"Argentina": "Argentina",
	"Armenia": "Armaniston",
	"Australia": "Avstraliya",
	"Austria": "Avstriya",
	"Azerbaijan": "O'zbekiston",
	"Bahamas": "Bagama orollari",
	"Bahrain": "Bahrayn",
	"Bangladesh": "Bangladesh",
	"Belgium": "Belgiya",
	"Bolivia": "Boliviya",
	"Bosnia and Herzegovina": "Bosniya va Gertsogovina",
	"Brazil": "Braziliya",
	"Bulgaria": "Bolgariya",
	"Cambodia": "Kambodja",
	"Cameroon": "Kamerun",
	"Canada": "Kanada",
	"Chile": "Chili",
	"China": "Xitoy",
	"Colombia": "Kolumbiya",
	"Costa Rica": "Kosta Rika",
	"Croatia": "Xorvatiya",
	"Cuba": "Kuba",
	"Czech Republic": "Chexiya",
	"Czechia": "Chexiya",
	"Denmark": "Daniya",
	"Dominican Republic": "Dominikan Respublikasi",
	"Ecuador": "Ekvador",
	"Egypt": "Misr",
	"El Salvador": "Salvador",
	"Estonia": "Estoniya",
	"Finland": "Finlandiya",
	"France": "Fransiya",
	"Georgia": "Gruziya",
	"Germany": "Germaniya",
	"Ghana": "Gana",
	"Greece": "Gretsiya",
	"Guatemala": "Gvatemala",
	"Haiti": "Gaiti",
	"Honduras": "Gonduras",
	"Hong Kong": "Gonkong",
	"Hungary": "Vengriya",
	"Iceland": "Islandiya",
	"India": "Hindiston",
	"Indonesia": "Indoneziya",
	"Iran": "Eron",
	"Iraq": "Iroq",
	"Ireland": "Irlandiya",
	"Israel": "Isroil",
	"Italy": "Italiya",
	"Jamaica": "Yamayka",
	"Japan": "Yaponiya",
	"Jordan": "Iordaniya",
	"Kazakhstan": "Qozog'iston",
	"Kenya": "Keniya",
	"North Korea": "Shimoliy Koreya",
	"South Korea": "Janubiy Koreya",
	"Kuwait": "Quvayt",
	"Latvia": "Latviya",
	"Lebanon": "Livan",
	"Libya": "Liviya",
	"Lithuania": "Litva",
	"Luxembourg": "Lyuksemburg",
	"Malaysia": "Malayziya",
	"Malta": "Malta",
	"Mexico": "Meksika",
	"Moldova": "Moldova",
	"Monaco": "Monako",
	"Mongolia": "Mongoliya",
	"Morocco": "Marokash",
	"Nepal": "Nepal",
	"Netherlands": "Niderlandiya",
	"New Zealand": "Yangi Zelandiya",
	"Nicaragua": "Nikaragua",
	"Nigeria": "Nigeriya",
	"Norway": "Norvegiya",
	"Oman": "Ummon",
	"Pakistan": "Pokiston",
	"Palestine": "Falastin",
	"Panama": "Panama",
	"Paraguay": "Paragvay",
	"Peru": "Peru",
	"Philippines": "Filippin",
	"Poland": "Polsha",
	"Portugal": "Portugaliya",
	"Puerto Rico": "Puerto-Riko",
	"Qatar": "Qatar",
	"Romania": "Ruminiya",
	"Russia": "Rossiya",
	"Russian Federation": "Rossiya",
	"Saudi Arabia": "Saudiya Arabistoni",
	"Senegal": "Senegal",
	"Serbia": "Serbiya",
	"Singapore": "Singapur",
	"Slovakia": "Slovakiya",
	"Slovenia": "Sloveniya",
	"South Africa": "Janubiy Afrika",
	"Spain": "Ispaniya",
	"Sri Lanka": "Shri Lanka",
	"Sweden": "Shvetsiya",
	"Switzerland": "Shveytsariya",
	"Syria": "Suriya",
	"Taiwan": "Tayvan",
	"Thailand": "Tailand",
	"Tunisia": "Tunis",
	"Turkey": "Turkiya",
	"Ukraine": "Ukraina",
	"United Arab Emirates": "Birlashgan Arab Amirliklari",
	"United Kingdom": "Buyuk Britaniya",
	"United States of America": "Amerika Qo'shma Shtatlari",
	"United States": "Amerika Qo'shma Shtatlari",
	"USA": "Amerika Qo'shma Shtatlari",
	"Uruguay": "Urugvay",
	"Venezuela": "Venesuela",
	"Vietnam": "Vyetnam",
	"Yemen": "Yaman",
	"Zimbabwe": "Zimbabve",
};

export type Locale = "uz";

export interface LocalizedMovie {
	id?: string;
	title: string;
	description?: string;
	genre?: string[];
	country?: string;
	poster_url?: string;
	backdrop_url?: string;
	year?: number;
	duration?: number;
	quality?: string;
	slug?: string;

	// Localized fields from backend
	title_uz?: string;
	description_uz?: string;
	genres_uz?: string[];
	countries_uz?: string[];
	original_title?: string;
	tmdb_id?: number;
	metadata_source?: string;
}

/**
 * Get localized display title for a movie (always Uzbek)
 */
export function getLocalizedTitle(movie: LocalizedMovie | null, _locale: Locale = "uz"): string {
	if (!movie) return "";
	if (movie.title_uz) {
		return movie.title_uz;
	}
	return movie.title || "";
}

/**
 * Get localized display description for a movie (always Uzbek)
 */
export function getLocalizedDescription(movie: LocalizedMovie | null, _locale: Locale = "uz"): string {
	if (!movie) return "";
	if (movie.description_uz) {
		return movie.description_uz;
	}
	return movie.description || "";
}

/**
 * Get localized genres for a movie (always Uzbek)
 */
export function getLocalizedGenres(movie: LocalizedMovie | null, _locale: Locale = "uz"): string[] {
	if (!movie) return [];
	// First try backend-provided Uzbek genres
	if (movie.genres_uz && movie.genres_uz.length > 0) {
		return movie.genres_uz;
	}
	// Fall back to frontend mapping
	return localizeGenres(movie.genre || []);
}

/**
 * Get localized country for a movie (always Uzbek)
 */
export function getLocalizedCountry(movie: LocalizedMovie | null, _locale: Locale = "uz"): string {
	if (!movie) return "";
	// First try backend-provided Uzbek countries
	if (movie.countries_uz && movie.countries_uz.length > 0) {
		return movie.countries_uz[0];
	}
	// Fall back to frontend mapping
	if (movie.country) {
		return localizeSingleCountry(movie.country);
	}
	return "";
}

/**
 * Localize a single genre name to Uzbek.
 * Case-insensitive — accepts "drama", "Drama", "DRAMA" equally.
 * Unknown genres fall back to the input with first letter capitalized.
 */
export function localizeSingleGenre(genre: string): string {
	if (!genre) return "";
	const trimmed = genre.trim();
	const normalized = (trimmed
		.toLowerCase()
		.replace(/[_-]+/g, " ")
		.replace(/\s+/g, " ")
		.trim());
	const uz = UZBEK_GENRE_LOOKUP[normalized];
	if (uz) return uz;
	return trimmed.charAt(0).toUpperCase() + trimmed.slice(1);
}

/**
 * Localize a slice of genre names to Uzbek
 */
export function localizeGenres(genres: string[]): string[] {
	if (!genres || genres.length === 0) return [];
	return genres.map(g => localizeSingleGenre(g));
}

/**
 * Localize a single country name to Uzbek
 */
export function localizeSingleCountry(country: string): string {
	if (!country) return "";
	return UZBEK_COUNTRY_MAP[country] || country;
}

/**
 * Localize a slice of country names to Uzbek
 */
export function localizeCountries(countries: string[]): string[] {
	if (!countries || countries.length === 0) return [];
	return countries.map(c => localizeSingleCountry(c));
}
