package domain

import "time"

type DeviceToken struct {
	UserID    string
	Token     string
	Platform  string // ios/android
	Timezone  string
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Notification struct {
	UserID string
	Title  string
	Body   string
	Token  string
}
