package repository

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"viv/internal/core/usecase"
)

var _ usecase.AccountDataStore = (*FirestoreAccountDataStore)(nil)

// userSubcollections lists every subcollection nested under
// users/{userID}/... across this codebase — verified against every
// repository file's own Collection(...) calls, not guessed. Account
// deletion must sweep every one of these: Firestore never
// cascade-deletes subcollections when their parent document is
// deleted, unlike Neon's foreign keys.
//
// "consents" is deliberately NOT here: it's an append-only legal audit
// trail (GDPR Art. 7.1 — demonstrating consent was given, not just
// asserting it) and must survive account deletion indefinitely, unlike
// everything else a user can produce. See NeonAccountAnonymizer's doc
// comment for the Neon-side equivalent of this carve-out.
var userSubcollections = []string{
	"checkins",
	"plans",
	"plan_jobs",
	"change_lifestyle",
	"daily_checkins",
	"session_logs",
	"exercise_pins",
	"nutrition_plan",
	"recovery_actions",
	"weekly_plan_drafts",
}

// FirestoreAccountDataStore implements the "hard delete everything"
// half of account deletion — see usecase.DeleteAccountUseCase's doc
// comment for why this is treated as fatal/required rather than
// best-effort like the Neon side.
type FirestoreAccountDataStore struct {
	client *firestore.Client
}

func NewFirestoreAccountDataStore(client *firestore.Client) *FirestoreAccountDataStore {
	return &FirestoreAccountDataStore{client: client}
}

func (s *FirestoreAccountDataStore) DeleteAllUserData(ctx context.Context, userID string) error {
	userDoc := s.client.Collection("users").Doc(userID)

	for _, sub := range userSubcollections {
		if err := deleteAllDocs(ctx, s.client, userDoc.Collection(sub)); err != nil {
			return fmt.Errorf("deleting users/%s/%s: %w", userID, sub, err)
		}
	}

	// device_tokens is a top-level collection keyed by user ID
	// (see FirestoreDeviceTokenRepository.Upsert), not nested under
	// users/{userID} like everything else — it needs its own delete.
	if _, err := s.client.Collection("device_tokens").Doc(userID).Delete(ctx); err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("deleting device_tokens/%s: %w", userID, err)
	}

	if _, err := userDoc.Delete(ctx); err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("deleting users/%s: %w", userID, err)
	}

	return nil
}

// deleteAllDocs removes every document in col. The Go Admin SDK has no
// built-in recursive-collection-delete (unlike the Node SDK's
// recursiveDelete) — BulkWriter is the documented way to do this:
// enqueue deletes, let it rate-limit internally, call End() to flush
// and block until every one of them actually completes, then check
// each job for a real per-document error before looping for the next
// page. Re-queries rather than paging by cursor, since a doc already
// deleted simply stops appearing in the next page's results.
func deleteAllDocs(ctx context.Context, client *firestore.Client, col *firestore.CollectionRef) error {
	for {
		docs, err := col.Limit(300).Documents(ctx).GetAll()
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			return nil
		}

		bulkWriter := client.BulkWriter(ctx)
		jobs := make([]*firestore.BulkWriterJob, 0, len(docs))
		for _, d := range docs {
			job, err := bulkWriter.Delete(d.Ref)
			if err != nil {
				return fmt.Errorf("enqueue delete %s: %w", d.Ref.Path, err)
			}
			jobs = append(jobs, job)
		}
		bulkWriter.End()

		for _, job := range jobs {
			if _, err := job.Results(); err != nil {
				return fmt.Errorf("delete failed: %w", err)
			}
		}
	}
}
