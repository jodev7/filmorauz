package models

// RequiredChannel represents a Telegram channel that user must subscribe to
type RequiredChannel struct {
	Key   string // Env key suffix (e.g., "anime", "movie")
	ID    int64  // Numeric chat ID (e.g., -1001234567890)
	URL   string // Join URL (e.g., https://t.me/anime_channel)
	Title string // Display title (e.g., "Anime kanal")
}

// SubscriptionStatus holds the result of subscription check
type SubscriptionStatus struct {
	IsSubscribed bool
	MissingChans []RequiredChannel
}
