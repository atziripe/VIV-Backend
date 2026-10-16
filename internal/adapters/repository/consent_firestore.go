package repository

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var (
	_ usecase.ConsentRepository = (*FirestoreConsentRepository)(nil)
	_ usecase.ConsentChecker    = (*FirestoreConsentRepository)(nil)
)

// FirestoreConsentRepository stores consent events under
// users/{userID}/consents/{autoID} — append-only: Grant always adds a new
// document, never overwrites one, so the full grant/revoke history is
// always there to point to (see domain.Consent's doc comment).
type FirestoreConsentRepository struct {
	client *firestore.Client
}

func NewFirestoreConsentRepository(client *firestore.Client) *FirestoreConsentRepository {
	return &FirestoreConsentRepository{client: client}
}

type consentDoc struct {
	Purpose       string    `firestore:"purpose"`
	Granted       bool      `firestore:"granted"`
	PolicyVersion string    `firestore:"policy_version"`
	CreatedAt     time.Time `firestore:"created_at"`
}

func (r *FirestoreConsentRepository) col(userID string) *firestore.CollectionRef {
	return r.client.Collection("users").Doc(userID).Collection("consents")
}

func (r *FirestoreConsentRepository) Grant(ctx context.Context, userID string, purpose domain.ConsentPurpose, granted bool, policyVersion string) error {
	doc := consentDoc{
		Purpose:       string(purpose),
		Granted:       granted,
		PolicyVersion: policyVersion,
		CreatedAt:     time.Now().UTC(),
	}
	_, _, err := r.col(userID).Add(ctx, doc)
	return err
}

// GetAll reduces the full history to the current state: each purpose's
// most recently created record. Reads the whole (small — a handful of
// purposes, rarely re-consented) collection rather than one query per
// purpose, same trade-off FirestoreLifestyleChangeRepository.ListByUser
// already makes.
func (r *FirestoreConsentRepository) GetAll(ctx context.Context, userID string) (map[domain.ConsentPurpose]domain.Consent, error) {
	iter := r.col(userID).Documents(ctx)
	defer iter.Stop()

	latest := map[domain.ConsentPurpose]domain.Consent{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var d consentDoc
		if err := doc.DataTo(&d); err != nil {
			return nil, err
		}

		purpose := domain.ConsentPurpose(d.Purpose)
		c := domain.Consent{
			UserID:        userID,
			Purpose:       purpose,
			Granted:       d.Granted,
			PolicyVersion: d.PolicyVersion,
			CreatedAt:     d.CreatedAt,
		}
		if existing, ok := latest[purpose]; !ok || c.CreatedAt.After(existing.CreatedAt) {
			latest[purpose] = c
		}
	}
	return latest, nil
}

func (r *FirestoreConsentRepository) IsGranted(ctx context.Context, userID string, purpose domain.ConsentPurpose) (bool, error) {
	all, err := r.GetAll(ctx, userID)
	if err != nil {
		return false, err
	}
	c, ok := all[purpose]
	return ok && c.Granted, nil
}
