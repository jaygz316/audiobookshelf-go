package handlers

type ListeningStatsItem struct {
	TimeListened  float64 `json:"timeListened"`
	Title         string  `json:"title"`
	Author        string  `json:"author"`
	MediaItemType string  `json:"mediaItemType"`
}

type PlaybackSessionResponse struct {
	ID            string  `json:"id"`
	UserID        string  `json:"userId"`
	Username      string  `json:"username"`
	MediaItemID   string  `json:"mediaItemId"`
	MediaItemType string  `json:"mediaItemType"`
	Title         string  `json:"title"`
	Author        string  `json:"author"`
	StartTime     float64 `json:"startTime"`
	TimeListened  float64 `json:"timeListened"`
	LastTime      float64 `json:"lastTime"`
	UpdatedAt     string  `json:"updatedAt"`
	PlayMethod    string  `json:"playMethod"`
	DeviceInfo    string  `json:"deviceInfo"`
}

type ListeningStatsResponse struct {
	TotalTime      float64                       `json:"totalTime"`
	Today          float64                       `json:"today"`
	Days           map[string]float64            `json:"days"`
	DayOfWeek      map[string]float64            `json:"dayOfWeek"`
	Items          map[string]ListeningStatsItem `json:"items"`
	TopAuthors     map[string]float64            `json:"topAuthors"`
	TopGenres      map[string]float64            `json:"topGenres"`
	RecentSessions []PlaybackSessionResponse     `json:"recentSessions"`
	ItemsFinished  int                           `json:"itemsFinished"`
	DaysListened   int                           `json:"daysListened"`
}

type ServerListeningStatsResponse struct {
	TotalTime      float64                       `json:"totalTime"`
	Today          float64                       `json:"today"`
	Days           map[string]float64            `json:"days"`
	DayOfWeek      map[string]float64            `json:"dayOfWeek"`
	Items          map[string]ListeningStatsItem `json:"items"`
	TopAuthors     map[string]float64            `json:"topAuthors"`
	TopGenres      map[string]float64            `json:"topGenres"`
	TopUsers       map[string]float64            `json:"topUsers"`
	RecentSessions []PlaybackSessionResponse     `json:"recentSessions"`
	ItemsFinished  int                           `json:"itemsFinished"`
	DaysListened   int                           `json:"daysListened"`
}
