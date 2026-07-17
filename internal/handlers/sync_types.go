package handlers

// LocalMediaProgressItem represents one item in the sync list
type LocalMediaProgressItem struct {
	ID                        interface{} `json:"id"`
	LibraryItemID             string      `json:"libraryItemId"`
	EpisodeID                 *string     `json:"episodeId"`
	Duration                  float64     `json:"duration"`
	Progress                  *float64    `json:"progress"`
	CurrentTime               *float64    `json:"currentTime"`
	IsFinished                bool        `json:"isFinished"`
	HideFromContinueListening bool        `json:"hideFromContinueListening"`
	UpdatedAt                 interface{} `json:"updatedAt"` // can be float64 or string
}

// LocalMediaProgressPayload is the payload of POST /api/me/sync-local-progress
type LocalMediaProgressPayload struct {
	LocalMediaProgress []LocalMediaProgressItem `json:"localMediaProgress"`
}

// LocalSessionItem represents a playback session to be synced
type LocalSessionItem struct {
	ID            string      `json:"id"`
	LibraryID     string      `json:"libraryId"`
	LibraryItemID string      `json:"libraryItemId"`
	EpisodeID     *string     `json:"episodeId"`
	TimeListening float64     `json:"timeListening"`
	StartTime     float64     `json:"startTime"`
	CurrentTime   float64     `json:"currentTime"`
	StartedAt     interface{} `json:"startedAt"`
	UpdatedAt     interface{} `json:"updatedAt"`
	Duration      float64     `json:"duration"`
	PlayMethod    interface{} `json:"playMethod"`
	MediaPlayer   string      `json:"mediaPlayer"`
	DeviceInfo    interface{} `json:"deviceInfo"`
}

// LocalSessionsPayload is the payload of POST /api/session/local-all
type LocalSessionsPayload struct {
	Sessions []LocalSessionItem `json:"sessions"`
}

// SyncSessionResult is returned for each session processed
type SyncSessionResult struct {
	ID             string `json:"id"`
	Success        bool   `json:"success"`
	ProgressSynced bool   `json:"progressSynced"`
}

// SyncSessionsResponse is the response of POST /api/session/local-all
type SyncSessionsResponse struct {
	Results []SyncSessionResult `json:"results"`
}
