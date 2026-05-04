package domain

import "time"

type DeviceToken struct {
	UserID    string
	Token     string
	Platform  string // ios/android
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
