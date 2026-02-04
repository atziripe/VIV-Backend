package domain

import "time"

type PlanJobStatus string

const (
	PlanJobQueued  PlanJobStatus = "queued"
	PlanJobRunning PlanJobStatus = "running"
	PlanJobDone    PlanJobStatus = "done"
	PlanJobFailed  PlanJobStatus = "failed"
)

type PlanJob struct {
	ID        string
	UserID    string
	Status    PlanJobStatus
	CheckinID string
	PlanID    string
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}
