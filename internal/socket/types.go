package socket

// PublicUser represents the public user structure sent on online/offline events.
type PublicUser struct {
	ID        string      `json:"id"`
	Username  string      `json:"username"`
	Type      string      `json:"type"`
	Session   interface{} `json:"session"` // can be nil
	LastSeen  int64       `json:"lastSeen"`
	CreatedAt int64       `json:"createdAt"`
}

// OnlineUser represents the online user structure for admins.
type OnlineUser struct {
	ID               string        `json:"id"`
	Username         string        `json:"username"`
	Type             string        `json:"type"`
	IsActive         bool          `json:"isActive"`
	Connections      int           `json:"connections"`
	LastSeen         int64         `json:"lastSeen"`
	Session          interface{}   `json:"session"` // can be nil
	PlaybackSessions []interface{} `json:"playbackSessions"`
}
