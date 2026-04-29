package domain

// RecoveryBanner is what the frontend renders.
type RecoveryBanner struct {
	Label   string `json:"label"`   // "Today — Wednesday · rest day"
	Title   string `json:"title"`   // "Yesterday's strength session is converting..."
	Body    string `json:"body"`    // 2 sentences, max 45 words
	Variant string `json:"variant"` // "O", "T", "N", "R" — for debugging/analytics
}

// RecoveryMove is a single actionable recovery move for today.
type RecoveryMove struct {
	MoveID string `json:"move_id"`
	Title  string `json:"title"`
	Timing string `json:"timing"`
	Detail string `json:"detail"`
	Impact string `json:"impact"`
}

// RecoveryDay is the complete recovery response — banner + moves.
type RecoveryDay struct {
	Banner RecoveryBanner `json:"banner"`
	Moves  []RecoveryMove `json:"moves"`
}
