package domain

import "time"

type Checkin struct {
	ID                 string
	UserID             string
	WeekStart          time.Time
	CreatedAt          time.Time
	SleepQuality       string
	BodyStatus         string
	Appetite           string
	StressLevel        string
	LastWeekFeeling    string
	CycleStart         *time.Time
	WorkloadPrediction string
	MentalEnergy       string
	PromptVersion      string
}
