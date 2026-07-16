package feed

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Database helper structs
type audioFile struct {
	Index    int     `json:"index"`
	Duration float64 `json:"duration"`
	Codec    string  `json:"codec"`
	MimeType string  `json:"mimeType"`
	Metadata struct {
		Path     string `json:"path"`
		RelPath  string `json:"relPath"`
		Filename string `json:"filename"`
		Ext      string `json:"ext"`
		Size     int64  `json:"size"`
	} `json:"metadata"`
}

type audiobookTrack struct {
	Index       int     `json:"index"`
	Exclude     bool    `json:"exclude"`
	Duration    float64 `json:"duration"`
	Codec       string  `json:"codec"`
	MimeType    string  `json:"mimeType"`
	StartOffset float64 `json:"startOffset"`
	Metadata    struct {
		Path     string `json:"path"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"metadata"`
}

type audiobookChapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

type podcastEpData struct {
	ID          string
	Title       string
	AudioFile   string
	PubDate     string
	Description string
	Season      string
	Episode     string
	EpisodeType string
}

func (m *FeedManager) checkUserAccess(ctx context.Context, userID, libraryID string) (bool, error) {
	var userType string
	var isActive int
	var permissionsStr sql.NullString
	err := m.db.QueryRowContext(ctx, "SELECT type, isActive, permissions FROM users WHERE id = ?", userID).Scan(&userType, &isActive, &permissionsStr)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("user not found: %w", err)
	}
	if err != nil {
		return false, fmt.Errorf("query user permissions: %w", err)
	}
	if isActive == 0 {
		return false, fmt.Errorf("user is inactive")
	}
	if userType == "root" || userType == "admin" {
		return true, nil
	}
	if !permissionsStr.Valid || permissionsStr.String == "" {
		return false, nil
	}

	type userPermissions struct {
		AccessAllLibraries  *bool    `json:"accessAllLibraries"`
		LibrariesAccessible []string `json:"librariesAccessible"`
	}
	var perm userPermissions
	if err := json.Unmarshal([]byte(permissionsStr.String), &perm); err != nil {
		return false, fmt.Errorf("unmarshal user permissions: %w", err)
	}

	if perm.AccessAllLibraries != nil && *perm.AccessAllLibraries {
		return true, nil
	}
	for _, lid := range perm.LibrariesAccessible {
		if lid == libraryID {
			return true, nil
		}
	}
	return false, nil
}

func hasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) bool {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dType string
		var notnull int
		var dfltVal sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dType, &notnull, &dfltVal, &pk); err == nil {
			if name == columnName {
				return true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false
	}
	return false
}

func queryPodcastEpisode(ctx context.Context, db *sql.DB, epID string) (*podcastEpData, error) {
	hasPubDate := hasColumn(ctx, db, "podcastEpisodes", "pubDate")
	hasDesc := hasColumn(ctx, db, "podcastEpisodes", "description")
	hasSeason := hasColumn(ctx, db, "podcastEpisodes", "season")
	hasEp := hasColumn(ctx, db, "podcastEpisodes", "episode")
	hasEpType := hasColumn(ctx, db, "podcastEpisodes", "episodeType")

	query := "SELECT id, title, audioFile"
	if hasPubDate {
		query += ", pubDate"
	}
	if hasDesc {
		query += ", description"
	}
	if hasSeason {
		query += ", season"
	}
	if hasEp {
		query += ", episode"
	}
	if hasEpType {
		query += ", episodeType"
	}
	query += " FROM podcastEpisodes WHERE id = ?"

	row := db.QueryRowContext(ctx, query, epID)

	var id, title, audioFileStr string
	dest := []interface{}{&id, &title, &audioFileStr}

	var pubDateVal, descVal, seasonVal, epVal, epTypeVal sql.NullString
	if hasPubDate {
		dest = append(dest, &pubDateVal)
	}
	if hasDesc {
		dest = append(dest, &descVal)
	}
	if hasSeason {
		dest = append(dest, &seasonVal)
	}
	if hasEp {
		dest = append(dest, &epVal)
	}
	if hasEpType {
		dest = append(dest, &epTypeVal)
	}

	if err := row.Scan(dest...); err != nil {
		return nil, fmt.Errorf("scan podcast episode: %w", err)
	}

	ep := &podcastEpData{
		ID:        id,
		Title:     title,
		AudioFile: audioFileStr,
	}
	if pubDateVal.Valid {
		ep.PubDate = pubDateVal.String
	}
	if descVal.Valid {
		ep.Description = descVal.String
	}
	if seasonVal.Valid {
		ep.Season = seasonVal.String
	}
	if epVal.Valid {
		ep.Episode = epVal.String
	}
	if epTypeVal.Valid {
		ep.EpisodeType = epTypeVal.String
	}
	return ep, nil
}

func queryPodcastEpisodes(ctx context.Context, db *sql.DB, podcastID string) ([]*podcastEpData, error) {
	hasPubDate := hasColumn(ctx, db, "podcastEpisodes", "pubDate")
	hasDesc := hasColumn(ctx, db, "podcastEpisodes", "description")
	hasSeason := hasColumn(ctx, db, "podcastEpisodes", "season")
	hasEp := hasColumn(ctx, db, "podcastEpisodes", "episode")
	hasEpType := hasColumn(ctx, db, "podcastEpisodes", "episodeType")

	query := "SELECT id, title, audioFile"
	if hasPubDate {
		query += ", pubDate"
	}
	if hasDesc {
		query += ", description"
	}
	if hasSeason {
		query += ", season"
	}
	if hasEp {
		query += ", episode"
	}
	if hasEpType {
		query += ", episodeType"
	}
	query += " FROM podcastEpisodes WHERE podcastId = ?"

	rows, err := db.QueryContext(ctx, query, podcastID)
	if err != nil {
		return nil, fmt.Errorf("query podcast episodes: %w", err)
	}
	defer rows.Close()

	var eps []*podcastEpData
	for rows.Next() {
		var id, title, audioFileStr string
		dest := []interface{}{&id, &title, &audioFileStr}
		var pubDateVal, descVal, seasonVal, epVal, epTypeVal sql.NullString
		if hasPubDate {
			dest = append(dest, &pubDateVal)
		}
		if hasDesc {
			dest = append(dest, &descVal)
		}
		if hasSeason {
			dest = append(dest, &seasonVal)
		}
		if hasEp {
			dest = append(dest, &epVal)
		}
		if hasEpType {
			dest = append(dest, &epTypeVal)
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan podcast episode row: %w", err)
		}

		ep := &podcastEpData{
			ID:        id,
			Title:     title,
			AudioFile: audioFileStr,
		}
		if pubDateVal.Valid {
			ep.PubDate = pubDateVal.String
		}
		if descVal.Valid {
			ep.Description = descVal.String
		}
		if seasonVal.Valid {
			ep.Season = seasonVal.String
		}
		if epVal.Valid {
			ep.Episode = epVal.String
		}
		if epTypeVal.Valid {
			ep.EpisodeType = epTypeVal.String
		}
		eps = append(eps, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("podcast episodes rows error: %w", err)
	}
	return eps, nil
}
