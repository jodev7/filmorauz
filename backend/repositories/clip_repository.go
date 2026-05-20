package repositories

import (
	"context"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ClipRepository struct {
	col       *mongo.Collection
	moviesCol *mongo.Collection // for $lookup-style genre enrichment
	seriesCol *mongo.Collection
}

func NewClipRepository(db *mongo.Database) *ClipRepository {
	return &ClipRepository{
		col:       db.Collection("clips"),
		moviesCol: db.Collection("movies"),
		seriesCol: db.Collection("series"),
	}
}

func (r *ClipRepository) Collection() *mongo.Collection {
	return r.col
}

func (r *ClipRepository) Create(ctx context.Context, clip *models.Clip) error {
	clip.CreatedAt = time.Now()
	result, err := r.col.InsertOne(ctx, clip)
	if err != nil {
		return err
	}
	clip.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *ClipRepository) CreateMany(ctx context.Context, clips []models.Clip) error {
	if len(clips) == 0 {
		return nil
	}
	docs := make([]interface{}, len(clips))
	now := time.Now()
	for i := range clips {
		clips[i].CreatedAt = now
		docs[i] = clips[i]
	}
	_, err := r.col.InsertMany(ctx, docs)
	return err
}

func (r *ClipRepository) FindByMovieID(ctx context.Context, movieID primitive.ObjectID) ([]models.Clip, error) {
	cursor, err := r.col.Find(ctx, bson.M{"movie_id": movieID}, options.Find().SetSort(bson.M{"sequence": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		return nil, err
	}
	return clips, nil
}

func (r *ClipRepository) List(ctx context.Context, limit, offset int64) ([]models.Clip, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	filter := bson.M{}

	// Debug: log database and collection info
	log.Printf("[ClipRepo] List: db=%s, coll=%s", r.col.Database().Name(), r.col.Name())

	total, countErr := r.col.CountDocuments(ctx, filter)
	if countErr != nil {
		log.Printf("[ClipRepo] List: CountDocuments error=%v", countErr)
	} else {
		log.Printf("[ClipRepo] List: total clips in DB=%d, limit=%d, offset=%d", total, limit, offset)
	}

	// Try raw bson.M to see if there's a struct mismatch
	var rawDocs []bson.M
	rawCursor, rawErr := r.col.Find(ctx, filter)
	if rawErr != nil {
		log.Printf("[ClipRepo] List: Raw Find error=%v", rawErr)
	} else {
		if err := rawCursor.All(ctx, &rawDocs); err != nil {
			log.Printf("[ClipRepo] List: Raw decode error=%v", err)
		} else {
			log.Printf("[ClipRepo] List: Raw query returned %d docs", len(rawDocs))
			for i, doc := range rawDocs {
				log.Printf("[ClipRepo] List: doc[%d] _id=%v, movie_id=%v", i, doc["_id"], doc["movie_id"])
				if i >= 2 {
					break // Only log first 3
				}
			}
		}
		rawCursor.Close(ctx)
	}

	opts := options.Find().
		SetSort(bson.M{"created_at": -1}).
		SetLimit(limit).
		SetSkip(offset)

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		log.Printf("[ClipRepo] List: Find error=%v", err)
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		log.Printf("[ClipRepo] List: Decode error=%v", err)
		return nil, 0, err
	}

	log.Printf("[ClipRepo] List: returned %d clips (countErr=%v)", len(clips), countErr)

	if countErr != nil {
		total = int64(len(clips))
	}

	return clips, total, nil
}

// ListFiltered returns clips matching the given filter (movie / series /
// episode scope) with sequence-ordered output and limit/offset pagination.
// Used to lazy-load a single content group's clips from the admin UI.
func (r *ClipRepository) ListFiltered(ctx context.Context, filter bson.M, limit, offset int64) ([]models.Clip, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	total, _ := r.col.CountDocuments(ctx, filter)
	opts := options.Find().
		SetSort(bson.D{{Key: "sequence", Value: 1}, {Key: "clip_index", Value: 1}, {Key: "_id", Value: 1}}).
		SetLimit(limit).
		SetSkip(offset)
	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		return nil, 0, err
	}
	return clips, total, nil
}

// MovieClipGroup is a per-movie summary returned by GroupClips.
type MovieClipGroup struct {
	GroupKey         string               `json:"group_key"`
	MovieID          primitive.ObjectID   `json:"movie_id"`
	Title            string               `json:"title"`
	Slug             string               `json:"slug"`
	Code             string               `json:"code"`
	Genre            []string             `json:"genre,omitempty"`
	ClipCount        int                  `json:"clip_count"`
	IGUploadedCount  int                  `json:"ig_uploaded_count"`
	LastIGUploadAt   *time.Time           `json:"last_ig_upload_at,omitempty"`
	MatchMovieIDs    []primitive.ObjectID `json:"match_movie_ids,omitempty"`
	MatchMovieCodes  []string             `json:"match_movie_codes,omitempty"`
	MatchMovieSlugs  []string             `json:"match_movie_slugs,omitempty"`
	MatchMovieTitles []string             `json:"match_movie_titles,omitempty"`
}

// EpisodeClipGroup is a per-episode summary inside a SeasonClipGroup.
type EpisodeClipGroup struct {
	GroupKey        string               `json:"group_key"`
	EpisodeID       primitive.ObjectID   `json:"episode_id"`
	EpisodeNumber   int                  `json:"episode_number"`
	Title           string               `json:"title"`
	ClipCount       int                  `json:"clip_count"`
	IGUploadedCount int                  `json:"ig_uploaded_count"`
	LastIGUploadAt  *time.Time           `json:"last_ig_upload_at,omitempty"`
	MatchEpisodeIDs []primitive.ObjectID `json:"match_episode_ids,omitempty"`
}

// SeasonClipGroup nests episodes under a season number.
type SeasonClipGroup struct {
	SeasonNumber int                `json:"season_number"`
	ClipCount    int                `json:"clip_count"`
	Episodes     []EpisodeClipGroup `json:"episodes"`
}

// SeriesClipGroup is a per-series summary including nested seasons/episodes.
type SeriesClipGroup struct {
	GroupKey          string               `json:"group_key"`
	SeriesID          primitive.ObjectID   `json:"series_id"`
	Title             string               `json:"title"`
	Slug              string               `json:"slug"`
	Genre             []string             `json:"genre,omitempty"`
	ClipCount         int                  `json:"clip_count"`
	IGUploadedCount   int                  `json:"ig_uploaded_count"`
	LastIGUploadAt    *time.Time           `json:"last_ig_upload_at,omitempty"`
	Seasons           []SeasonClipGroup    `json:"seasons"`
	MatchSeriesIDs    []primitive.ObjectID `json:"match_series_ids,omitempty"`
	MatchSeriesSlugs  []string             `json:"match_series_slugs,omitempty"`
	MatchSeriesTitles []string             `json:"match_series_titles,omitempty"`
}

// normalizeGroupKey returns a lowercase, whitespace-collapsed form of s.
// Used so "Vonpis " and "vonpis" group together when ObjectIDs disagree.
func normalizeGroupKey(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

// firstNonEmpty picks the first non-empty trimmed string, used to surface a
// human label even when individual clips disagree on title/slug.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

var (
	sxxexxPrefixRE      = regexp.MustCompile(`(?i)^\s*s\d+\s*e\d+\s*[-:._]?\s*`)
	qismPrefixRE        = regexp.MustCompile(`(?i)^\s*\d+\s*-\s*qism\s*[-:._]?\s*`)
	genericSeriesNameRE = regexp.MustCompile(`(?i)^(serial|serials|seriallar|series)$`)
)

func isGenericSeriesLabel(s string) bool {
	s = normalizeGroupKey(s)
	return s != "" && genericSeriesNameRE.MatchString(s)
}

func cleanSeriesTitleCandidate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || isGenericSeriesLabel(s) {
		return ""
	}
	return s
}

func cleanSeriesSlugCandidate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || isGenericSeriesLabel(s) {
		return ""
	}
	return s
}

func cleanEpisodeTitleForSeries(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = sxxexxPrefixRE.ReplaceAllString(s, "")
	s = qismPrefixRE.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	if isGenericSeriesLabel(s) {
		return ""
	}
	return s
}

func displayTitleFromSlug(slug string) string {
	slug = cleanSeriesSlugCandidate(slug)
	if slug == "" {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(slug, "-", " "))
}

func appendUniqueObjectID(dst []primitive.ObjectID, id primitive.ObjectID) []primitive.ObjectID {
	if id.IsZero() {
		return dst
	}
	for _, existing := range dst {
		if existing == id {
			return dst
		}
	}
	return append(dst, id)
}

func appendUniqueString(dst []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return dst
	}
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}

func clipFolderFromLocation(pathOrURL string) string {
	pathOrURL = strings.TrimSpace(pathOrURL)
	if pathOrURL == "" {
		return ""
	}
	const marker = "videos/clips/"
	if idx := strings.Index(pathOrURL, marker); idx != -1 {
		rest := pathOrURL[idx+len(marker):]
		if slash := strings.Index(rest, "/"); slash != -1 {
			return strings.TrimSpace(rest[:slash])
		}
		return strings.TrimSpace(rest)
	}
	parts := strings.Split(pathOrURL, "/")
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[len(parts)-2])
	}
	return ""
}

func clipClassificationStages() mongo.Pipeline {
	zero := primitive.NilObjectID
	return mongo.Pipeline{
		{{Key: "$addFields", Value: bson.M{
			"episode_id_effective": bson.M{"$ifNull": []interface{}{"$episode_id", zero}},
			"series_id_effective":  bson.M{"$ifNull": []interface{}{"$series_id", zero}},
			"season_id_effective":  bson.M{"$ifNull": []interface{}{"$season_id", zero}},
			"movie_id_effective":   bson.M{"$ifNull": []interface{}{"$movie_id", zero}},
			"movie_code_trim":      bson.M{"$trim": bson.M{"input": bson.M{"$ifNull": []interface{}{"$movie_code", ""}}}},
			"series_slug_trim":     bson.M{"$trim": bson.M{"input": bson.M{"$ifNull": []interface{}{"$series_slug", ""}}}},
			"series_title_trim":    bson.M{"$trim": bson.M{"input": bson.M{"$ifNull": []interface{}{"$series_title", ""}}}},
			"source_type_trim":     bson.M{"$trim": bson.M{"input": bson.M{"$ifNull": []interface{}{"$source_type", ""}}}},
			"content_kind_trim":    bson.M{"$trim": bson.M{"input": bson.M{"$ifNull": []interface{}{"$content_kind", ""}}}},
		}}},
		{{Key: "$addFields", Value: bson.M{
			"has_episode_ref": bson.M{"$ne": []interface{}{"$episode_id_effective", zero}},
			"has_series_ref":  bson.M{"$ne": []interface{}{"$series_id_effective", zero}},
			"has_season_ref": bson.M{"$or": []interface{}{
				bson.M{"$ne": []interface{}{"$season_id_effective", zero}},
				bson.M{"$gt": []interface{}{bson.M{"$ifNull": []interface{}{"$season_number", 0}}, 0}},
			}},
			"has_movie_ref": bson.M{"$or": []interface{}{
				bson.M{"$ne": []interface{}{"$movie_id_effective", zero}},
				bson.M{"$ne": []interface{}{"$movie_code_trim", ""}},
				bson.M{"$eq": []interface{}{"$source_type_trim", "movie"}},
				bson.M{"$eq": []interface{}{"$content_kind_trim", "movie"}},
			}},
			"has_series_text": bson.M{"$or": []interface{}{
				bson.M{"$ne": []interface{}{"$series_slug_trim", ""}},
				bson.M{"$ne": []interface{}{"$series_title_trim", ""}},
			}},
		}}},
		{{Key: "$addFields", Value: bson.M{
			"is_series": bson.M{"$or": []interface{}{
				bson.M{"$eq": []interface{}{"$content_kind_trim", "series"}},
				bson.M{"$eq": []interface{}{"$source_type_trim", "series_episode"}},
				"$has_episode_ref",
				"$has_series_ref",
				"$has_season_ref",
				"$has_series_text",
			}},
		}}},
		{{Key: "$addFields", Value: bson.M{
			"is_movie": bson.M{"$and": []interface{}{
				bson.M{"$eq": []interface{}{"$is_series", false}},
				bson.M{"$or": []interface{}{
					"$has_movie_ref",
					bson.M{"$and": []interface{}{
						bson.M{"$eq": []interface{}{"$has_episode_ref", false}},
						bson.M{"$eq": []interface{}{"$has_season_ref", false}},
						bson.M{"$or": []interface{}{
							bson.M{"$eq": []interface{}{"$has_series_ref", false}},
							bson.M{"$eq": []interface{}{"$has_series_text", false}},
						}},
					}},
				}},
			}},
		}}},
	}
}

// GroupClips builds a (movies, series→season→episode) summary tree from the
// clips collection. Only counts/metadata are returned — never the clip docs
// themselves — so the response stays small even with 100k+ clips.
func (r *ClipRepository) GroupClips(ctx context.Context) ([]MovieClipGroup, []SeriesClipGroup, int64, error) {
	totalClips, _ := r.col.CountDocuments(ctx, bson.M{})
	classify := clipClassificationStages()

	igFlag := bson.M{"$cond": []interface{}{
		bson.M{"$gt": []interface{}{"$instagram_upload_count", 0}}, 1, 0,
	}}

	// ── Movies ────────────────────────────────────────────────────────
	moviePipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"is_movie": true}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"movie_id":    "$movie_id",
				"movie_code":  "$movie_code",
				"movie_slug":  "$movie_slug",
				"movie_title": "$movie_title",
			},
			"title":             bson.M{"$first": "$movie_title"},
			"slug":              bson.M{"$first": "$movie_slug"},
			"code":              bson.M{"$first": "$movie_code"},
			"path":              bson.M{"$first": "$path"},
			"url":               bson.M{"$first": "$url"},
			"filename":          bson.M{"$first": "$filename"},
			"clip_count":        bson.M{"$sum": 1},
			"ig_uploaded_count": bson.M{"$sum": igFlag},
			"last_ig_upload_at": bson.M{"$max": "$last_instagram_upload_at"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "title", Value: 1}}}},
	}
	moviePipeline = append(classify, moviePipeline...)

	movies := []MovieClipGroup{}
	{
		cur, err := r.col.Aggregate(ctx, moviePipeline)
		if err != nil {
			return nil, nil, 0, err
		}
		var rows []bson.M
		if err := cur.All(ctx, &rows); err != nil {
			cur.Close(ctx)
			return nil, nil, 0, err
		}
		cur.Close(ctx)
		// Dedupe by canonical string key: movie_id, else movie_code, else
		// normalized slug/title. Mongo's $group already collapses by movie_id,
		// but legacy clips can lack one or have inconsistent ids — without
		// this pass those rows either get dropped (zero id) or split into
		// duplicate groups.
		movieIdx := map[string]int{}
		for _, row := range rows {
			keyMap, _ := row["_id"].(bson.M)
			id, _ := keyMap["movie_id"].(primitive.ObjectID)
			title := asString(row["title"])
			slug := asString(row["slug"])
			code := asString(row["code"])
			folder := clipFolderFromLocation(firstNonEmpty(asString(row["path"]), asString(row["url"])))
			filename := asString(row["filename"])

			var key string
			switch {
			case !id.IsZero():
				key = "id:" + id.Hex()
			case strings.TrimSpace(code) != "":
				key = "code:" + strings.TrimSpace(code)
			case normalizeGroupKey(slug) != "":
				key = "slug:" + normalizeGroupKey(slug)
			case normalizeGroupKey(folder) != "":
				key = "folder:" + normalizeGroupKey(folder)
			case normalizeGroupKey(title) != "":
				key = "title:" + normalizeGroupKey(title)
			case normalizeGroupKey(filename) != "":
				key = "file:" + normalizeGroupKey(filename)
			default:
				continue
			}

			clipCount := asInt(row["clip_count"])
			igCount := asInt(row["ig_uploaded_count"])
			lastIG := asTimePtr(row["last_ig_upload_at"])

			if i, ok := movieIdx[key]; ok {
				movies[i].ClipCount += clipCount
				movies[i].IGUploadedCount += igCount
				if lastIG != nil && (movies[i].LastIGUploadAt == nil || lastIG.After(*movies[i].LastIGUploadAt)) {
					movies[i].LastIGUploadAt = lastIG
				}
				if movies[i].MovieID.IsZero() && !id.IsZero() {
					movies[i].MovieID = id
				}
				if movies[i].Title == "" {
					movies[i].Title = firstNonEmpty(title, folder, filename)
				}
				if movies[i].Slug == "" {
					movies[i].Slug = slug
				}
				if movies[i].Code == "" {
					movies[i].Code = code
				}
				movies[i].MatchMovieIDs = appendUniqueObjectID(movies[i].MatchMovieIDs, id)
				movies[i].MatchMovieCodes = appendUniqueString(movies[i].MatchMovieCodes, code)
				movies[i].MatchMovieSlugs = appendUniqueString(movies[i].MatchMovieSlugs, slug)
				movies[i].MatchMovieTitles = appendUniqueString(movies[i].MatchMovieTitles, title)
				continue
			}
			movieIdx[key] = len(movies)
			movies = append(movies, MovieClipGroup{
				GroupKey:         key,
				MovieID:          id,
				Title:            firstNonEmpty(title, folder, filename),
				Slug:             slug,
				Code:             code,
				ClipCount:        clipCount,
				IGUploadedCount:  igCount,
				LastIGUploadAt:   lastIG,
				MatchMovieIDs:    appendUniqueObjectID(nil, id),
				MatchMovieCodes:  appendUniqueString(nil, code),
				MatchMovieSlugs:  appendUniqueString(nil, slug),
				MatchMovieTitles: appendUniqueString(nil, title),
			})
		}
		sort.Slice(movies, func(i, j int) bool { return strings.ToLower(movies[i].Title) < strings.ToLower(movies[j].Title) })
	}

	// ── Series → Season → Episode ─────────────────────────────────────
	episodePipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"is_series": true}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"series_id":     "$series_id",
				"season_number": "$season_number",
				"episode_id":    "$episode_id",
			},
			"content_kind":      bson.M{"$first": "$content_kind"},
			"source_type":       bson.M{"$first": "$source_type"},
			"series_title":      bson.M{"$first": "$series_title"},
			"series_slug":       bson.M{"$first": "$series_slug"},
			"episode_number":    bson.M{"$first": "$episode_number"},
			"episode_title":     bson.M{"$first": "$title"},
			"clip_count":        bson.M{"$sum": 1},
			"ig_uploaded_count": bson.M{"$sum": igFlag},
			"last_ig_upload_at": bson.M{"$max": "$last_instagram_upload_at"},
		}}},
	}
	episodePipeline = append(classify, episodePipeline...)

	// Group key priority: series_id > normalized series_slug > normalized
	// series_title. This collapses Vonpis variants whose ObjectIDs differ
	// (legacy ingestion sometimes emitted distinct ids for the same series).
	type seasonBuild struct {
		group      *SeasonClipGroup
		episodeIdx map[string]int
	}

	seriesIdx := map[string]*SeriesClipGroup{}
	seriesAlias := map[string]string{}
	seasonIdx := map[string]map[int]*seasonBuild{}
	seriesOrder := []string{}
	seriesMetaCache := map[primitive.ObjectID]struct {
		Title string
		Slug  string
	}{}
	{
		cur, err := r.col.Aggregate(ctx, episodePipeline)
		if err != nil {
			return nil, nil, 0, err
		}
		var rows []bson.M
		if err := cur.All(ctx, &rows); err != nil {
			cur.Close(ctx)
			return nil, nil, 0, err
		}
		cur.Close(ctx)
		for _, row := range rows {
			key, _ := row["_id"].(bson.M)
			if key == nil {
				continue
			}
			sID, _ := key["series_id"].(primitive.ObjectID)
			seriesTitle := cleanSeriesTitleCandidate(asString(row["series_title"]))
			seriesSlug := cleanSeriesSlugCandidate(asString(row["series_slug"]))
			contentKind := asString(row["content_kind"])
			sourceType := asString(row["source_type"])
			epTitle := cleanEpisodeTitleForSeries(asString(row["episode_title"]))

			if !sID.IsZero() {
				if _, ok := seriesMetaCache[sID]; !ok {
					var doc struct {
						Title string `bson:"title"`
						Slug  string `bson:"slug"`
					}
					findErr := r.col.Database().Collection("series").FindOne(ctx, bson.M{"_id": sID}).Decode(&doc)
					if findErr == nil {
						seriesMetaCache[sID] = struct {
							Title string
							Slug  string
						}{
							Title: cleanSeriesTitleCandidate(doc.Title),
							Slug:  cleanSeriesSlugCandidate(doc.Slug),
						}
					} else {
						seriesMetaCache[sID] = struct {
							Title string
							Slug  string
						}{}
					}
				}
				meta := seriesMetaCache[sID]
				seriesTitle = firstNonEmpty(meta.Title, seriesTitle)
				seriesSlug = firstNonEmpty(meta.Slug, seriesSlug)
			}

			var primaryKey string
			switch {
			case !sID.IsZero():
				primaryKey = "id:" + sID.Hex()
			case normalizeGroupKey(seriesSlug) != "":
				primaryKey = "slug:" + normalizeGroupKey(seriesSlug)
			case normalizeGroupKey(seriesTitle) != "":
				primaryKey = "title:" + normalizeGroupKey(seriesTitle)
			default:
				continue
			}
			groupKey := primaryKey
			titleAlias := ""
			if normalizeGroupKey(seriesTitle) != "" && (contentKind == "series" || sourceType == "series_episode") {
				titleAlias = "title:" + normalizeGroupKey(seriesTitle)
			}
			if titleAlias != "" {
				if canonical, ok := seriesAlias[titleAlias]; ok {
					groupKey = canonical
				}
			}
			if canonical, ok := seriesAlias[primaryKey]; ok {
				groupKey = canonical
			}

			seasonNum := asInt(key["season_number"])
			epID, _ := key["episode_id"].(primitive.ObjectID)
			epNum := asInt(row["episode_number"])
			clipCount := asInt(row["clip_count"])
			igCount := asInt(row["ig_uploaded_count"])
			lastIG := asTimePtr(row["last_ig_upload_at"])

			sg := seriesIdx[groupKey]
			if sg == nil {
				sg = &SeriesClipGroup{
					GroupKey:          groupKey,
					SeriesID:          sID,
					Title:             seriesTitle,
					Slug:              seriesSlug,
					MatchSeriesIDs:    appendUniqueObjectID(nil, sID),
					MatchSeriesSlugs:  appendUniqueString(nil, seriesSlug),
					MatchSeriesTitles: appendUniqueString(nil, seriesTitle),
				}
				seriesIdx[groupKey] = sg
				seasonIdx[groupKey] = map[int]*seasonBuild{}
				seriesOrder = append(seriesOrder, groupKey)
				seriesAlias[primaryKey] = groupKey
				if titleAlias != "" {
					seriesAlias[titleAlias] = groupKey
				}
			} else {
				if sg.SeriesID.IsZero() && !sID.IsZero() {
					sg.SeriesID = sID
				}
				sg.Title = firstNonEmpty(sg.Title, seriesTitle)
				sg.Slug = firstNonEmpty(sg.Slug, seriesSlug)
				sg.MatchSeriesIDs = appendUniqueObjectID(sg.MatchSeriesIDs, sID)
				sg.MatchSeriesSlugs = appendUniqueString(sg.MatchSeriesSlugs, seriesSlug)
				sg.MatchSeriesTitles = appendUniqueString(sg.MatchSeriesTitles, seriesTitle)
				seriesAlias[primaryKey] = groupKey
				if titleAlias != "" {
					seriesAlias[titleAlias] = groupKey
				}
			}
			sg.ClipCount += clipCount
			sg.IGUploadedCount += igCount
			if lastIG != nil && (sg.LastIGUploadAt == nil || lastIG.After(*sg.LastIGUploadAt)) {
				sg.LastIGUploadAt = lastIG
			}

			seasonMap := seasonIdx[groupKey]
			seasonGroup := seasonMap[seasonNum]
			if seasonGroup == nil {
				seasonGroup = &seasonBuild{
					group:      &SeasonClipGroup{SeasonNumber: seasonNum},
					episodeIdx: map[string]int{},
				}
				seasonMap[seasonNum] = seasonGroup
			}
			seasonGroup.group.ClipCount += clipCount

			episodePrimaryKey := ""
			switch {
			case !epID.IsZero():
				episodePrimaryKey = "id:" + epID.Hex()
			case epNum > 0 && normalizeGroupKey(epTitle) != "":
				episodePrimaryKey = "numtitle:" + strconv.Itoa(epNum) + ":" + normalizeGroupKey(epTitle)
			case epNum > 0:
				episodePrimaryKey = "num:" + strconv.Itoa(epNum)
			default:
				episodePrimaryKey = "title:" + normalizeGroupKey(epTitle)
			}

			episodeMergeKey := episodePrimaryKey
			if epNum > 0 && normalizeGroupKey(epTitle) != "" {
				episodeMergeKey = "numtitle:" + strconv.Itoa(epNum) + ":" + normalizeGroupKey(epTitle)
			}
			if idx, ok := seasonGroup.episodeIdx[episodeMergeKey]; ok {
				seasonGroup.group.Episodes[idx].ClipCount += clipCount
				seasonGroup.group.Episodes[idx].IGUploadedCount += igCount
				if lastIG != nil && (seasonGroup.group.Episodes[idx].LastIGUploadAt == nil || lastIG.After(*seasonGroup.group.Episodes[idx].LastIGUploadAt)) {
					seasonGroup.group.Episodes[idx].LastIGUploadAt = lastIG
				}
				if seasonGroup.group.Episodes[idx].EpisodeID.IsZero() && !epID.IsZero() {
					seasonGroup.group.Episodes[idx].EpisodeID = epID
				}
				if seasonGroup.group.Episodes[idx].Title == "" {
					seasonGroup.group.Episodes[idx].Title = epTitle
				}
				seasonGroup.group.Episodes[idx].MatchEpisodeIDs = appendUniqueObjectID(seasonGroup.group.Episodes[idx].MatchEpisodeIDs, epID)
				continue
			}
			seasonGroup.episodeIdx[episodeMergeKey] = len(seasonGroup.group.Episodes)
			seasonGroup.group.Episodes = append(seasonGroup.group.Episodes, EpisodeClipGroup{
				GroupKey:        episodePrimaryKey,
				EpisodeID:       epID,
				EpisodeNumber:   epNum,
				Title:           epTitle,
				ClipCount:       clipCount,
				IGUploadedCount: igCount,
				LastIGUploadAt:  lastIG,
				MatchEpisodeIDs: appendUniqueObjectID(nil, epID),
			})
		}
	}

	series := make([]SeriesClipGroup, 0, len(seriesIdx))
	for _, groupKey := range seriesOrder {
		sg := seriesIdx[groupKey]
		seasonMap := seasonIdx[groupKey]
		seasonNums := make([]int, 0, len(seasonMap))
		for n := range seasonMap {
			seasonNums = append(seasonNums, n)
		}
		sort.Ints(seasonNums)
		seasons := make([]SeasonClipGroup, 0, len(seasonNums))
		for _, n := range seasonNums {
			s := seasonMap[n].group
			sort.Slice(s.Episodes, func(i, j int) bool {
				if s.Episodes[i].EpisodeNumber == s.Episodes[j].EpisodeNumber {
					return strings.ToLower(s.Episodes[i].Title) < strings.ToLower(s.Episodes[j].Title)
				}
				return s.Episodes[i].EpisodeNumber < s.Episodes[j].EpisodeNumber
			})
			seasons = append(seasons, *s)
		}
		if sg.Title == "" {
			for _, season := range seasons {
				for _, episode := range season.Episodes {
					if title := cleanEpisodeTitleForSeries(episode.Title); title != "" {
						sg.Title = title
						break
					}
				}
				if sg.Title != "" {
					break
				}
			}
		}
		if sg.Title == "" {
			sg.Title = displayTitleFromSlug(sg.Slug)
		}
		sg.Seasons = seasons
		series = append(series, *sg)
	}
	sort.Slice(series, func(i, j int) bool { return strings.ToLower(series[i].Title) < strings.ToLower(series[j].Title) })

	return movies, series, totalClips, nil
}

func (r *ClipRepository) ClipGroupsDebug(ctx context.Context, sampleLimit int64) (bson.M, error) {
	if sampleLimit <= 0 {
		sampleLimit = 20
	}

	counts := bson.M{
		"total_clips": r.mustCount(ctx, bson.M{}),
		"episode_id_exists": r.mustCount(ctx, bson.M{
			"episode_id": bson.M{"$exists": true, "$nin": bson.A{nil, primitive.NilObjectID}},
		}),
		"series_id_exists": r.mustCount(ctx, bson.M{
			"series_id": bson.M{"$exists": true, "$nin": bson.A{nil, primitive.NilObjectID}},
		}),
		"movie_id_exists": r.mustCount(ctx, bson.M{
			"movie_id": bson.M{"$exists": true, "$nin": bson.A{nil, primitive.NilObjectID}},
		}),
		"movie_code_exists": r.mustCount(ctx, bson.M{
			"movie_code": bson.M{"$exists": true, "$nin": bson.A{nil, ""}},
		}),
		"no_series_id_and_no_episode_id": r.mustCount(ctx, bson.M{
			"$and": bson.A{
				bson.M{"$or": bson.A{
					bson.M{"series_id": bson.M{"$exists": false}},
					bson.M{"series_id": nil},
					bson.M{"series_id": primitive.NilObjectID},
				}},
				bson.M{"$or": bson.A{
					bson.M{"episode_id": bson.M{"$exists": false}},
					bson.M{"episode_id": nil},
					bson.M{"episode_id": primitive.NilObjectID},
				}},
			},
		}),
	}

	sampleFilter := bson.M{
		"$and": bson.A{
			bson.M{"$or": bson.A{
				bson.M{"series_id": bson.M{"$exists": false}},
				bson.M{"series_id": nil},
				bson.M{"series_id": primitive.NilObjectID},
			}},
			bson.M{"$or": bson.A{
				bson.M{"episode_id": bson.M{"$exists": false}},
				bson.M{"episode_id": nil},
				bson.M{"episode_id": primitive.NilObjectID},
			}},
			bson.M{"$or": bson.A{
				bson.M{"season_id": bson.M{"$exists": false}},
				bson.M{"season_id": nil},
				bson.M{"season_id": primitive.NilObjectID},
			}},
		},
	}
	projection := bson.M{
		"_id":          1,
		"title":        1,
		"filename":     1,
		"path":         1,
		"url":          1,
		"movie_id":     1,
		"movie_code":   1,
		"movie_slug":   1,
		"series_id":    1,
		"season_id":    1,
		"episode_id":   1,
		"source_type":  1,
		"content_kind": 1,
	}
	opts := options.Find().SetLimit(sampleLimit)
	cur, err := r.col.Find(ctx, sampleFilter, opts.SetProjection(projection))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}

	movies, series, total, err := r.GroupClips(ctx)
	if err != nil {
		return nil, err
	}

	return bson.M{
		"counts":             counts,
		"sample_docs":        docs,
		"movie_group_count":  len(movies),
		"series_group_count": len(series),
		"total_groups":       len(movies) + len(series),
		"total_clips":        total,
		"movies_preview":     clipMoviePreview(movies, 5),
		"series_preview":     clipSeriesPreview(series, 5),
	}, nil
}

func (r *ClipRepository) mustCount(ctx context.Context, filter interface{}) int64 {
	n, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		log.Printf("[ClipRepo] debug count failed filter=%v err=%v", filter, err)
		return 0
	}
	return n
}

func clipMoviePreview(movies []MovieClipGroup, limit int) []bson.M {
	if limit > len(movies) {
		limit = len(movies)
	}
	out := make([]bson.M, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, bson.M{
			"title":      movies[i].Title,
			"group_key":  movies[i].GroupKey,
			"clip_count": movies[i].ClipCount,
			"code":       movies[i].Code,
			"slug":       movies[i].Slug,
			"movie_id":   movies[i].MovieID,
		})
	}
	return out
}

func clipSeriesPreview(series []SeriesClipGroup, limit int) []bson.M {
	if limit > len(series) {
		limit = len(series)
	}
	out := make([]bson.M, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, bson.M{
			"title":        series[i].Title,
			"group_key":    series[i].GroupKey,
			"clip_count":   series[i].ClipCount,
			"season_count": len(series[i].Seasons),
			"series_id":    series[i].SeriesID,
			"series_slug":  series[i].Slug,
		})
	}
	return out
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func asTimePtr(v interface{}) *time.Time {
	switch t := v.(type) {
	case time.Time:
		if t.IsZero() {
			return nil
		}
		return &t
	case primitive.DateTime:
		tt := t.Time()
		if tt.IsZero() {
			return nil
		}
		return &tt
	}
	return nil
}

func (r *ClipRepository) DeleteByMovieID(ctx context.Context, movieID primitive.ObjectID) error {
	_, err := r.col.DeleteMany(ctx, bson.M{"movie_id": movieID})
	return err
}

func (r *ClipRepository) FindBySeriesID(ctx context.Context, seriesID primitive.ObjectID) ([]models.Clip, error) {
	cursor, err := r.col.Find(ctx, bson.M{"series_id": seriesID}, options.Find().SetSort(bson.M{"sequence": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		return nil, err
	}
	return clips, nil
}

func (r *ClipRepository) DeleteBySeriesID(ctx context.Context, seriesID primitive.ObjectID) error {
	_, err := r.col.DeleteMany(ctx, bson.M{"series_id": seriesID})
	return err
}

// FindByEpisodeID returns clips linked to a single episode. Used during
// series cascade delete so legacy clip rows that only carry episode_id
// (no series_id) are still picked up.
func (r *ClipRepository) FindByEpisodeID(ctx context.Context, episodeID primitive.ObjectID) ([]models.Clip, error) {
	cursor, err := r.col.Find(ctx, bson.M{"episode_id": episodeID}, options.Find().SetSort(bson.M{"sequence": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		return nil, err
	}
	return clips, nil
}

// DeleteBySeriesAndEpisodeIDs removes every clip whose series_id matches
// or whose episode_id is in the given list. The OR query catches legacy
// clip rows that lack a series_id but reference one of the deleted
// episodes — without it those rows would survive a series delete and
// keep pointing at vanished content.
func (r *ClipRepository) DeleteBySeriesAndEpisodeIDs(ctx context.Context, seriesID primitive.ObjectID, episodeIDs []primitive.ObjectID) error {
	filter := bson.M{"series_id": seriesID}
	if len(episodeIDs) > 0 {
		filter = bson.M{
			"$or": []bson.M{
				{"series_id": seriesID},
				{"episode_id": bson.M{"$in": episodeIDs}},
			},
		}
	}
	_, err := r.col.DeleteMany(ctx, filter)
	return err
}

func (r *ClipRepository) CountByMovieID(ctx context.Context, movieID primitive.ObjectID) (int64, error) {
	return r.col.CountDocuments(ctx, bson.M{"movie_id": movieID})
}

// RecordInstagramUpload increments the upload counter and sets status/timestamp.
// status must be "success" or "failed".
func (r *ClipRepository) RecordInstagramUpload(ctx context.Context, clipID primitive.ObjectID, status string) error {
	now := time.Now()
	uploaded := status == "success"
	_, err := r.col.UpdateOne(ctx,
		bson.M{"_id": clipID},
		bson.M{
			"$inc": bson.M{"instagram_upload_count": 1},
			"$set": bson.M{
				"uploaded_to_instagram":        uploaded,
				"last_instagram_upload_at":     now,
				"last_instagram_upload_status": status,
			},
		},
	)
	return err
}

// FindByID returns a single clip by its ObjectID.
func (r *ClipRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*models.Clip, error) {
	var clip models.Clip
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&clip)
	if err != nil {
		return nil, err
	}
	return &clip, nil
}
