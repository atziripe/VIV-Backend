package repository

import (
	"context"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// Compile-time check that FirestorePlanJobsRepository implements the interface.
var _ usecase.PlanJobsRepository = (*FirestorePlanJobsRepository)(nil)

// FirestorePlanJobsRepository stores plan generation jobs under:
// users/{userId}/plan_jobs/{jobId}
type FirestorePlanJobsRepository struct {
	client *firestore.Client
}

func NewFirestorePlanJobsRepository(client *firestore.Client) *FirestorePlanJobsRepository {
	return &FirestorePlanJobsRepository{client: client}
}

// Firestore DTO (document shape)
type planJobDoc struct {
	Status    string    `firestore:"status"`
	CheckinID *string   `firestore:"checkin_id,omitempty"`
	PlanID    *string   `firestore:"plan_id,omitempty"`
	Error     *string   `firestore:"error,omitempty"`
	CreatedAt time.Time `firestore:"created_at"`
	UpdatedAt time.Time `firestore:"updated_at"`
}

func (r *FirestorePlanJobsRepository) jobsCol(userID string) *firestore.CollectionRef {
	return r.client.Collection("users").Doc(userID).Collection("plan_jobs")
}

// CreateQueued creates a new job doc with status="queued" and returns its ID.
func (r *FirestorePlanJobsRepository) CreateQueued(ctx context.Context, userID, checkinID string) (string, error) {
	now := time.Now().UTC()

	var checkinPtr *string
	if strings.TrimSpace(checkinID) != "" {
		v := strings.TrimSpace(checkinID)
		checkinPtr = &v
	}

	doc := planJobDoc{
		Status:    string(domain.PlanJobQueued),
		CheckinID: checkinPtr,
		CreatedAt: now,
		UpdatedAt: now,
	}

	ref := r.jobsCol(userID).NewDoc()
	if _, err := ref.Set(ctx, doc); err != nil {
		return "", err
	}
	return ref.ID, nil
}

// MarkRunning sets status="running".
func (r *FirestorePlanJobsRepository) MarkRunning(ctx context.Context, userID, jobID string) error {
	_, err := r.jobsCol(userID).Doc(jobID).Update(ctx, []firestore.Update{
		{Path: "status", Value: string(domain.PlanJobRunning)},
		{Path: "updated_at", Value: time.Now().UTC()},
	})
	return err
}

// MarkDone sets status="done" and stores plan_id. Also clears error (if any).
func (r *FirestorePlanJobsRepository) MarkDone(ctx context.Context, userID, jobID, planID string) error {
	p := strings.TrimSpace(planID)
	_, err := r.jobsCol(userID).Doc(jobID).Update(ctx, []firestore.Update{
		{Path: "status", Value: string(domain.PlanJobDone)},
		{Path: "plan_id", Value: p},
		{Path: "error", Value: firestore.Delete},
		{Path: "updated_at", Value: time.Now().UTC()},
	})
	return err
}

// MarkFailed sets status="failed" and stores a human-readable error message.
func (r *FirestorePlanJobsRepository) MarkFailed(ctx context.Context, userID, jobID, errorMsg string) error {
	e := strings.TrimSpace(errorMsg)
	if e == "" {
		e = "unknown error"
	}
	_, err := r.jobsCol(userID).Doc(jobID).Update(ctx, []firestore.Update{
		{Path: "status", Value: string(domain.PlanJobFailed)},
		{Path: "error", Value: e},
		{Path: "updated_at", Value: time.Now().UTC()},
	})
	return err
}

// GetByID returns (nil, nil) when the document does not exist.
func (r *FirestorePlanJobsRepository) GetByID(ctx context.Context, userID, jobID string) (*domain.PlanJob, error) {
	doc, err := r.jobsCol(userID).Doc(jobID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var d planJobDoc
	if err := doc.DataTo(&d); err != nil {
		return nil, err
	}

	out := &domain.PlanJob{
		ID:        doc.Ref.ID,
		UserID:    userID,
		Status:    domain.PlanJobStatus(d.Status),
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}

	if d.CheckinID != nil {
		out.CheckinID = *d.CheckinID
	}
	if d.PlanID != nil {
		out.PlanID = *d.PlanID
	}
	if d.Error != nil {
		out.Error = *d.Error
	}

	return out, nil
}
