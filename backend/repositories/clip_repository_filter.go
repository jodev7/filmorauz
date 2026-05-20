package repositories

import (
	"context"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ClipGroupFilter is the union of filter / sort / pagination options the
// admin clips page can pass to GroupClipsFiltered. Every field is optional;
// the zero value reproduces the legacy GroupClips behaviour (all groups,
// no search, sort by title).
type ClipGroupFilter struct {
	Kind         string   // "movie" | "series" | "" (both)
	Query        string   // case-insensitive substring match against title/slug/code
	Genres       []string // lowercase genre slugs; OR semantics
	OnlyUnposted bool     // hide groups where every clip is already on IG
	Sort         string   // "title" (default) | "newest" | "most_clips" | "least_posted"
	Limit        int      // 0 ⇒ no limit
	Offset       int

	// SeriesSlugs / MovieSlugs restrict the result to a specific set of
	// content slugs. Used by the per-IG-account view in the admin page,
	// where each account is bound to a set of shows/genres.
	SeriesSlugs []string
	MovieSlugs  []string
}

// ClipGroupResult is the wire-level response shape returned by
// GroupClipsFiltered. TotalMovies / TotalSeries reflect counts after
// filters but before pagination, so the UI can render a stable "23 ta
// natija" badge even while paging.
type ClipGroupResult struct {
	Movies         []MovieClipGroup  `json:"movies"`
	Series         []SeriesClipGroup `json:"series"`
	TotalClips     int64             `json:"total_clips"`
	TotalMovies    int               `json:"total_movies"`    // filtered, pre-page
	TotalSeries    int               `json:"total_series"`    // filtered, pre-page
	TotalFiltered  int               `json:"total_filtered"`  // movies + series after filter
	AllGenres      []string          `json:"all_genres"`      // unique sorted genre slugs across all groups
	GroupCountMovies int             `json:"movie_group_count"` // unfiltered count
	GroupCountSeries int             `json:"series_group_count"`
}

// GroupClipsFiltered runs the heavyweight GroupClips aggregation, then
// enriches each group with its content's genre (one bulk fetch from the
// movies/series collections — cheap even for 1k contents), applies the
// caller's filters/sort, and finally paginates.
//
// Filtering in memory is intentional: the legacy GroupClips pipeline
// already loads the full result set, the response is tiny per-group
// (counts only), and pushing filters into the existing 200-line
// aggregation would multiply its surface area for very small gains at
// the scale we run at.
func (r *ClipRepository) GroupClipsFiltered(ctx context.Context, opts ClipGroupFilter) (*ClipGroupResult, error) {
	movies, series, totalClips, err := r.GroupClips(ctx)
	if err != nil {
		return nil, err
	}

	// Bulk-fetch genres so every group gets enriched in one round-trip
	// per collection. Anything we can't resolve falls back to an empty
	// slice — the UI then treats that group as "no genre".
	movieIDs := make([]primitive.ObjectID, 0, len(movies))
	movieSlugs := make([]string, 0, len(movies))
	for _, m := range movies {
		if !m.MovieID.IsZero() {
			movieIDs = append(movieIDs, m.MovieID)
		}
		if s := strings.TrimSpace(m.Slug); s != "" {
			movieSlugs = append(movieSlugs, s)
		}
	}
	seriesIDs := make([]primitive.ObjectID, 0, len(series))
	seriesSlugs := make([]string, 0, len(series))
	for _, s := range series {
		if !s.SeriesID.IsZero() {
			seriesIDs = append(seriesIDs, s.SeriesID)
		}
		if sl := strings.TrimSpace(s.Slug); sl != "" {
			seriesSlugs = append(seriesSlugs, sl)
		}
	}
	movieGenres := r.fetchGenres(ctx, r.moviesCol, movieIDs, movieSlugs)
	seriesGenres := r.fetchGenres(ctx, r.seriesCol, seriesIDs, seriesSlugs)

	for i := range movies {
		movies[i].Genre = lookupGenre(movieGenres, movies[i].MovieID, movies[i].Slug)
	}
	for i := range series {
		series[i].Genre = lookupGenre(seriesGenres, series[i].SeriesID, series[i].Slug)
	}

	// Collect the universe of genre slugs BEFORE filtering — the chip
	// selector in the UI needs every possible value, not just the ones
	// surviving the current query.
	allGenres := collectGenres(movies, series)

	// Apply filters.
	filteredMovies := filterMovieGroups(movies, opts)
	filteredSeries := filterSeriesGroups(series, opts)

	// Apply sort (movies and series sorted independently — they get
	// concatenated client-side or further sliced server-side by tab).
	sortMovieGroups(filteredMovies, opts.Sort)
	sortSeriesGroups(filteredSeries, opts.Sort)

	totalMovies := len(filteredMovies)
	totalSeries := len(filteredSeries)

	// Apply tab-aware pagination. When `kind` is set we paginate only
	// that slice; otherwise the page covers the concatenated list with
	// movies first (matches the existing UI ordering).
	pagedMovies, pagedSeries := paginateGroups(filteredMovies, filteredSeries, opts)

	return &ClipGroupResult{
		Movies:           pagedMovies,
		Series:           pagedSeries,
		TotalClips:       totalClips,
		TotalMovies:      totalMovies,
		TotalSeries:      totalSeries,
		TotalFiltered:    totalMovies + totalSeries,
		AllGenres:        allGenres,
		GroupCountMovies: len(movies),
		GroupCountSeries: len(series),
	}, nil
}

// AccountUploadStats summarises how many clips a given IG account has
// uploaded today / this week / this month. We log uploads in the
// clip_upload_history sub-collection that records each (clip_id,
// account, uploaded_at). When that sub-collection is absent we fall
// back to zeros so the admin UI still renders.
type AccountUploadStats struct {
	Account string `json:"account"`
	Today   int64  `json:"today"`
	Week    int64  `json:"week"`
	Month   int64  `json:"month"`
}

// AccountUploadStats counts last_instagram_upload_at on clips matching
// the given account. This is an approximation — only the most recent
// upload per clip is recorded — but it's accurate enough for the dash.
func (r *ClipRepository) AccountUploadStats(ctx context.Context, account string) (*AccountUploadStats, error) {
	now := time.Now()
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startWeek := startToday.AddDate(0, 0, -int(now.Weekday()))
	startMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	count := func(after time.Time) int64 {
		filter := bson.M{
			"last_instagram_upload_status": "success",
			"last_instagram_upload_at":     bson.M{"$gte": after},
		}
		// last_instagram_upload_account is not persisted today, so we
		// can't filter by account precisely. The dashboard treats
		// these counters as "uploads from any account" until we
		// extend the upload-tracking flow to record the account.
		_ = account
		n, _ := r.col.CountDocuments(ctx, filter)
		return n
	}

	return &AccountUploadStats{
		Account: account,
		Today:   count(startToday),
		Week:    count(startWeek),
		Month:   count(startMonth),
	}, nil
}

// ListAllGenres returns the unique sorted list of genres present on any
// movie or series document. Used by the admin clips filter chip
// selector. Cached at the caller layer if needed; here we re-read each
// call (cheap aggregation, infrequent admin endpoint).
func (r *ClipRepository) ListAllGenres(ctx context.Context) ([]string, error) {
	out := make(map[string]struct{})
	for _, col := range []*mongo.Collection{r.moviesCol, r.seriesCol} {
		cur, err := col.Distinct(ctx, "genre", bson.M{})
		if err != nil {
			return nil, err
		}
		for _, v := range cur {
			if s, ok := v.(string); ok {
				s = strings.ToLower(strings.TrimSpace(s))
				if s != "" {
					out[s] = struct{}{}
				}
			}
		}
	}
	genres := make([]string, 0, len(out))
	for g := range out {
		genres = append(genres, g)
	}
	sort.Strings(genres)
	return genres, nil
}

// --- internal helpers ---------------------------------------------------

type genreEntry struct {
	ByID   map[primitive.ObjectID][]string
	BySlug map[string][]string
}

// fetchGenres reads the (id, slug, genre) projection from the given
// collection for every id/slug we have. Indexes on _id and slug make
// this O(1) per row.
func (r *ClipRepository) fetchGenres(ctx context.Context, col *mongo.Collection, ids []primitive.ObjectID, slugs []string) genreEntry {
	entry := genreEntry{
		ByID:   map[primitive.ObjectID][]string{},
		BySlug: map[string][]string{},
	}
	if len(ids) == 0 && len(slugs) == 0 {
		return entry
	}

	or := make([]bson.M, 0, 2)
	if len(ids) > 0 {
		or = append(or, bson.M{"_id": bson.M{"$in": ids}})
	}
	if len(slugs) > 0 {
		or = append(or, bson.M{"slug": bson.M{"$in": slugs}})
	}
	filter := bson.M{"$or": or}
	if len(or) == 1 {
		filter = or[0]
	}

	cur, err := col.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1, "slug": 1, "genre": 1}))
	if err != nil {
		return entry
	}
	defer cur.Close(ctx)

	var rows []struct {
		ID    primitive.ObjectID `bson:"_id"`
		Slug  string             `bson:"slug"`
		Genre []string           `bson:"genre"`
	}
	_ = cur.All(ctx, &rows)
	for _, row := range rows {
		normalised := normalizeGenreList(row.Genre)
		if !row.ID.IsZero() {
			entry.ByID[row.ID] = normalised
		}
		if s := strings.TrimSpace(row.Slug); s != "" {
			entry.BySlug[s] = normalised
		}
	}
	return entry
}

func lookupGenre(entry genreEntry, id primitive.ObjectID, slug string) []string {
	if !id.IsZero() {
		if g, ok := entry.ByID[id]; ok {
			return g
		}
	}
	if s := strings.TrimSpace(slug); s != "" {
		if g, ok := entry.BySlug[s]; ok {
			return g
		}
	}
	return nil
}

func normalizeGenreList(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, g := range raw {
		g = strings.ToLower(strings.TrimSpace(g))
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	return out
}

func collectGenres(movies []MovieClipGroup, series []SeriesClipGroup) []string {
	seen := make(map[string]struct{})
	for _, m := range movies {
		for _, g := range m.Genre {
			seen[g] = struct{}{}
		}
	}
	for _, s := range series {
		for _, g := range s.Genre {
			seen[g] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for g := range seen {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

func filterMovieGroups(in []MovieClipGroup, opts ClipGroupFilter) []MovieClipGroup {
	if opts.Kind == "series" {
		return nil
	}
	out := make([]MovieClipGroup, 0, len(in))
	q := strings.ToLower(strings.TrimSpace(opts.Query))
	slugAllow := stringSet(opts.MovieSlugs)
	for _, m := range in {
		if q != "" && !stringMatches(q, m.Title, m.Slug, m.Code) {
			continue
		}
		if len(opts.Genres) > 0 && !hasAnyGenre(m.Genre, opts.Genres) {
			continue
		}
		if len(slugAllow) > 0 && !slugAllow[strings.TrimSpace(m.Slug)] {
			continue
		}
		if opts.OnlyUnposted && m.IGUploadedCount >= m.ClipCount {
			continue
		}
		out = append(out, m)
	}
	return out
}

func filterSeriesGroups(in []SeriesClipGroup, opts ClipGroupFilter) []SeriesClipGroup {
	if opts.Kind == "movie" {
		return nil
	}
	out := make([]SeriesClipGroup, 0, len(in))
	q := strings.ToLower(strings.TrimSpace(opts.Query))
	slugAllow := stringSet(opts.SeriesSlugs)
	for _, s := range in {
		if q != "" && !stringMatches(q, s.Title, s.Slug) {
			continue
		}
		if len(opts.Genres) > 0 && !hasAnyGenre(s.Genre, opts.Genres) {
			continue
		}
		if len(slugAllow) > 0 && !slugAllow[strings.TrimSpace(s.Slug)] {
			continue
		}
		if opts.OnlyUnposted && s.IGUploadedCount >= s.ClipCount {
			continue
		}
		out = append(out, s)
	}
	return out
}

func stringSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]bool, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out[v] = true
		}
	}
	return out
}

func stringMatches(q string, candidates ...string) bool {
	for _, c := range candidates {
		if strings.Contains(strings.ToLower(c), q) {
			return true
		}
	}
	return false
}

func hasAnyGenre(have, want []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, g := range have {
		for _, w := range want {
			if g == strings.ToLower(strings.TrimSpace(w)) {
				return true
			}
		}
	}
	return false
}

func sortMovieGroups(in []MovieClipGroup, mode string) {
	switch mode {
	case "newest":
		sort.SliceStable(in, func(i, j int) bool {
			return tsValue(in[i].LastIGUploadAt) > tsValue(in[j].LastIGUploadAt)
		})
	case "most_clips":
		sort.SliceStable(in, func(i, j int) bool { return in[i].ClipCount > in[j].ClipCount })
	case "least_posted":
		sort.SliceStable(in, func(i, j int) bool {
			a := in[i].ClipCount - in[i].IGUploadedCount
			b := in[j].ClipCount - in[j].IGUploadedCount
			return a > b
		})
	default:
		sort.SliceStable(in, func(i, j int) bool {
			return strings.ToLower(in[i].Title) < strings.ToLower(in[j].Title)
		})
	}
}

func sortSeriesGroups(in []SeriesClipGroup, mode string) {
	switch mode {
	case "newest":
		sort.SliceStable(in, func(i, j int) bool {
			return tsValue(in[i].LastIGUploadAt) > tsValue(in[j].LastIGUploadAt)
		})
	case "most_clips":
		sort.SliceStable(in, func(i, j int) bool { return in[i].ClipCount > in[j].ClipCount })
	case "least_posted":
		sort.SliceStable(in, func(i, j int) bool {
			a := in[i].ClipCount - in[i].IGUploadedCount
			b := in[j].ClipCount - in[j].IGUploadedCount
			return a > b
		})
	default:
		sort.SliceStable(in, func(i, j int) bool {
			return strings.ToLower(in[i].Title) < strings.ToLower(in[j].Title)
		})
	}
}

func tsValue(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.UnixNano()
}

func paginateGroups(movies []MovieClipGroup, series []SeriesClipGroup, opts ClipGroupFilter) ([]MovieClipGroup, []SeriesClipGroup) {
	if opts.Limit <= 0 {
		return movies, series
	}
	start := opts.Offset
	if start < 0 {
		start = 0
	}
	end := start + opts.Limit

	switch opts.Kind {
	case "movie":
		return sliceMovies(movies, start, end), nil
	case "series":
		return nil, sliceSeries(series, start, end)
	}

	// Combined view: movies first, then series. Pagination cuts across
	// the concatenated list so the boundary may split mid-list.
	totalMovies := len(movies)
	combinedEnd := end
	if combinedEnd > totalMovies+len(series) {
		combinedEnd = totalMovies + len(series)
	}
	outMovies := sliceMovies(movies, start, minInt(combinedEnd, totalMovies))
	seriesStart := maxInt(0, start-totalMovies)
	seriesEnd := maxInt(0, combinedEnd-totalMovies)
	outSeries := sliceSeries(series, seriesStart, seriesEnd)
	return outMovies, outSeries
}

func sliceMovies(in []MovieClipGroup, start, end int) []MovieClipGroup {
	if start >= len(in) {
		return nil
	}
	if end > len(in) {
		end = len(in)
	}
	if start < 0 {
		start = 0
	}
	if end <= start {
		return nil
	}
	return in[start:end]
}

func sliceSeries(in []SeriesClipGroup, start, end int) []SeriesClipGroup {
	if start >= len(in) {
		return nil
	}
	if end > len(in) {
		end = len(in)
	}
	if start < 0 {
		start = 0
	}
	if end <= start {
		return nil
	}
	return in[start:end]
}
