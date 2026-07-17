package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"net/http"

	"audiobookshelf/internal/core"
)

// opmlOutline represents a single outline element in an OPML document
type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	Type     string        `xml:"type,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	Outlines []opmlOutline `xml:"outline"`
}

// opmlDocument represents the full structure of an OPML file
type opmlDocument struct {
	XMLName  xml.Name      `xml:"opml"`
	Outlines []opmlOutline `xml:"body>outline"`
}

type opmlExportDocument struct {
	XMLName xml.Name       `xml:"opml"`
	Version string         `xml:"version,attr"`
	Head    opmlExportHead `xml:"head"`
	Body    opmlExportBody `xml:"body"`
}

type opmlExportHead struct {
	Title string `xml:"title"`
}

type opmlExportBody struct {
	Outlines []opmlExportOutline `xml:"outline"`
}

type opmlExportOutline struct {
	Text   string `xml:"text,attr"`
	Title  string `xml:"title,attr"`
	Type   string `xml:"type,attr"`
	XMLURL string `xml:"xmlUrl,attr"`
}

func findFeeds(outlines []opmlOutline) []map[string]string {
	var feeds []map[string]string
	for _, o := range outlines {
		if o.XMLURL != "" {
			title := o.Title
			if title == "" {
				title = o.Text
			}
			feeds = append(feeds, map[string]string{
				"title":   title,
				"feedUrl": o.XMLURL,
			})
		}
		if len(o.Outlines) > 0 {
			feeds = append(feeds, findFeeds(o.Outlines)...)
		}
	}
	return feeds
}

func handleParseOPML(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		if !user.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		var req struct {
			OPMLText string `json:"opmlText"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.OPMLText == "" {
			http.Error(w, `{"error": "opmlText parameter is required"}`, http.StatusBadRequest)
			return
		}

		var doc opmlDocument
		if err := xml.Unmarshal([]byte(req.OPMLText), &doc); err != nil {
			log.Errorf("[ParseOPML] Failed to parse XML: %v", err)
			http.Error(w, `{"error": "Failed to parse OPML XML"}`, http.StatusBadRequest)
			return
		}

		feeds := findFeeds(doc.Outlines)
		if feeds == nil {
			feeds = []map[string]string{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"feeds": feeds,
		})
	}
}

func handleExportOPML(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userVal := r.Context().Value(core.UserContextKey)
		if userVal == nil {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		user := userVal.(*core.UserSession)
		libraryID := r.URL.Query().Get("libraryId")
		if libraryID == "" {
			http.Error(w, `{"error": "libraryId parameter is required"}`, http.StatusBadRequest)
			return
		}
		if !user.CanAccessLibrary(libraryID) {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}
		initManagers(db)

		opmlText, err := globalFeedManager.GenerateOPML(r.Context(), user.ID, libraryID)
		if err != nil {
			log.Errorf("[Feed] GenerateOPML failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("Content-Disposition", "attachment; filename=podcasts.opml")
		w.Write([]byte(opmlText))
	}
}
