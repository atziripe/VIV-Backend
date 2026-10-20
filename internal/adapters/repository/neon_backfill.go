package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/api/iterator"

	"viv/internal/core/activity"
	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/mesocycle"
	"viv/internal/core/usecase"
)

// ============================================================================
// NEON BACKFILL — one-time copy of every historical Firestore record into
// Neon, entity by entity. This is the prerequisite for "Fase 4" (cutting
// reads over from Firestore to Neon, then decommissioning Firestore
// entirely): the dual-write path (dual_write.go) only ever mirrors data
// written *after* it went live for a given entity — nothing before that
// point was ever copied to Neon. Run this once and confirm the counts look
// right before touching any read path or removing any Firestore writer.
//
// Every write below reuses the exact same NeonXRepository method the live
// dual-write path already calls (same ON CONFLICT ... DO NOTHING/DO UPDATE
// idempotency), so this is safe to re-run end to end — including after a
// partial failure or an interrupted run. DryRun reads everything and tallies
// counts without writing to Neon at all, so you can sanity-check the numbers
// first.
//
// plan_jobs is deliberately NOT backfilled: it's transient operational
// state (queued/running/done/failed), nothing reads a user's historical
// job list, and NeonPlanJobsRepository has no method that preserves an
// arbitrary historical status (CreateQueuedLinked always writes "queued").
// Not a gap worth a special-case insert for data nothing downstream needs.
//
// consents IS backfilled (via NeonConsentRepository.CreateHistorical,
// which preserves each event's original created_at) — unlike plan_jobs,
// this is exactly the kind of historical record GDPR Art. 7.1 requires
// being able to produce, so losing anything before the dual-write went
// live for this entity is not an acceptable gap.
// ============================================================================

// BackfillReport tallies how many rows were written (or, in dry-run mode,
// would have been) per entity, plus any per-record errors — printed at the
// end so a human can sanity-check the counts before trusting the cutover.
type BackfillReport struct {
	Users            int
	Checkins         int
	Plans            int
	DeviceTokens     int
	LifestyleChanges int
	WeeklyPlanDrafts int
	ExercisePins     int
	DailyCheckins    int
	SessionLogs      int
	NutritionPlans   int
	RecoveryActions  int
	Consents         int
	Errors           []string
}

func RunNeonBackfill(ctx context.Context, fsClient *firestore.Client, pool *pgxpool.Pool, dryRun bool) (BackfillReport, error) {
	var report BackfillReport

	neonUsers := NewNeonUserRepository(pool)
	neonCheckins := NewNeonCheckinRepository(pool)
	neonPlans := NewNeonPlanRepository(pool)
	neonLifestyle := NewNeonLifestyleChangeRepository(pool)
	neonDrafts := NewNeonWeeklyPlanDraftRepository(pool)
	neonPins := NewNeonExercisePinRepository(pool)
	neonDailyCheckins := NewNeonDailyCheckinRepository(pool)
	neonSessionLogs := NewNeonSessionLogRepository(pool)
	neonNutrition := NewNeonNutritionPlanRepository(pool)
	neonRecovery := NewNeonRecoveryActionRepository(pool)
	neonDeviceTokens := NewNeonDeviceTokenRepository(pool)
	neonConsents := NewNeonConsentRepository(pool)

	// device_tokens is a top-level collection keyed by user ID, not nested
	// under users/{uid} like everything else — handled once, outside the
	// per-user loop below. GetAllActive (not a full collection scan) matches
	// what every live reader of this entity already treats as "all the
	// tokens that matter" — an inactive token has no backfill value.
	fsDeviceTokens := NewFirestoreDeviceTokenRepository(fsClient)
	tokens, err := fsDeviceTokens.GetAllActive(ctx)
	if err != nil {
		return report, fmt.Errorf("backfill: listing device tokens: %w", err)
	}
	for _, t := range tokens {
		report.DeviceTokens++
		if dryRun {
			continue
		}
		if err := neonDeviceTokens.Upsert(ctx, t); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("device_token user=%s: %v", t.UserID, err))
		}
	}

	iter := fsClient.Collection("users").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return report, fmt.Errorf("backfill: listing users: %w", err)
		}

		var u domain.User
		if err := doc.DataTo(&u); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("user %s: decode: %v", doc.Ref.ID, err))
			continue
		}
		u.ID = doc.Ref.ID
		report.Users++

		if !dryRun {
			// Every child entity below resolves its Neon FK via this user's
			// firebase_uid — if this write fails, skip the rest for this
			// user rather than inserting orphaned children that could
			// never resolve a user_id.
			if err := neonUsers.Save(ctx, &u); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("user %s: save: %v", u.ID, err))
				continue
			}
		}

		log.Printf("[backfill] user=%s", u.ID)

		backfillCheckins(ctx, fsClient, neonCheckins, u.ID, dryRun, &report)
		backfillPlans(ctx, fsClient, neonPlans, u.ID, dryRun, &report)
		backfillLifestyleChanges(ctx, fsClient, neonLifestyle, u.ID, dryRun, &report)
		backfillWeeklyPlanDrafts(ctx, fsClient, neonDrafts, u.ID, dryRun, &report)
		backfillExercisePins(ctx, fsClient, neonPins, u.ID, dryRun, &report)
		backfillDailyCheckins(ctx, fsClient, neonDailyCheckins, u.ID, dryRun, &report)
		backfillSessionLogs(ctx, fsClient, neonSessionLogs, u.ID, dryRun, &report)
		backfillNutritionPlan(ctx, fsClient, neonNutrition, u.ID, dryRun, &report)
		backfillRecoveryActions(ctx, fsClient, neonRecovery, u.ID, dryRun, &report)
		backfillConsents(ctx, fsClient, neonConsents, u.ID, dryRun, &report)
	}

	return report, nil
}

// -------------------- old pipeline: checkins (weekly) --------------------

func backfillCheckins(ctx context.Context, fsClient *firestore.Client, neon *NeonCheckinRepository, userID string, dryRun bool, report *BackfillReport) {
	docs, err := fsClient.Collection("users").Doc(userID).Collection("checkins").Documents(ctx).GetAll()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("checkins user=%s: list: %v", userID, err))
		return
	}
	for _, doc := range docs {
		var cd checkinDoc
		if err := doc.DataTo(&cd); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("checkins user=%s doc=%s: decode: %v", userID, doc.Ref.ID, err))
			continue
		}
		report.Checkins++
		if dryRun {
			continue
		}

		ch := &domain.Checkin{
			ID:              doc.Ref.ID,
			UserID:          userID,
			WeekStart:       cd.WeekStart,
			CreatedAt:       cd.CreatedAt,
			Sleep:           domain.CheckinSleep(cd.Sleep),
			Body:            domain.CheckinBody(cd.Body),
			AppetiteV2:      domain.CheckinAppetite(cd.AppetiteV2),
			Demand:          domain.CheckinDemand(cd.Demand),
			LastWeekFeel:    domain.CheckinLastWeek(cd.LastWeekFeel),
			CycleStart:      cd.CycleStart,
			PMSSymptoms:     (*domain.CheckinPMSSymptoms)(cd.PMSSymptoms),
			Predictability:  domain.CheckinPredictability(cd.Predictability),
			Readiness:       domain.CheckinReadiness(cd.Readiness),
			ModalitySkip:    cd.ModalitySkip,
			AdditionalNotes: cd.AdditionalNotes,
		}
		if err := neon.Create(ctx, ch); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("checkins user=%s doc=%s: write: %v", userID, doc.Ref.ID, err))
		}
	}
}

// -------------------- old pipeline: plans --------------------

func backfillPlans(ctx context.Context, fsClient *firestore.Client, neon *NeonPlanRepository, userID string, dryRun bool, report *BackfillReport) {
	docs, err := fsClient.Collection("users").Doc(userID).Collection("plans").Documents(ctx).GetAll()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("plans user=%s: list: %v", userID, err))
		return
	}
	for _, doc := range docs {
		var pd planDoc
		if err := doc.DataTo(&pd); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("plans user=%s doc=%s: decode: %v", userID, doc.Ref.ID, err))
			continue
		}
		report.Plans++
		if dryRun {
			continue
		}

		checkinID := ""
		if pd.CheckinID != nil {
			checkinID = *pd.CheckinID
		}
		p := &domain.Plan{
			ID:            doc.Ref.ID,
			UserID:        userID,
			Status:        pd.Status,
			CheckinID:     checkinID,
			CreatedAt:     pd.CreatedAt,
			StartDate:     pd.StartDate,
			EndDate:       pd.EndDate,
			CycleDayRange: pd.CycleDayRange,

			TrainingJSON:  []byte(pd.TrainingJSON),
			NutritionJSON: []byte(pd.NutritionJSON),
			RecoveryJSON:  []byte(pd.RecoveryJSON),

			TrainingCompleted: pd.TrainingCompleted,
			MealSelections:    pd.MealSelections,
			PhaseFeedback:     pd.PhaseFeedback,

			GeneratedFrom: pd.GeneratedFrom,
			SourceEventID: pd.SourceEventID,
			PlanVersion:   pd.PlanVersion,
		}
		if err := neon.Create(ctx, p); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("plans user=%s doc=%s: write: %v", userID, doc.Ref.ID, err))
		}
	}
}

// -------------------- lifestyle changes --------------------

func backfillLifestyleChanges(ctx context.Context, fsClient *firestore.Client, neon *NeonLifestyleChangeRepository, userID string, dryRun bool, report *BackfillReport) {
	fs := NewFirestoreLifestyleChangeRepository(fsClient)
	changes, err := fs.ListByUser(ctx, userID, 100000)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("lifestyle_changes user=%s: list: %v", userID, err))
		return
	}
	for _, c := range changes {
		report.LifestyleChanges++
		if dryRun {
			continue
		}
		if err := neon.Create(ctx, c); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("lifestyle_changes user=%s doc=%s: write: %v", userID, c.ID, err))
		}
	}
}

// -------------------- weekly plan drafts --------------------

func backfillWeeklyPlanDrafts(ctx context.Context, fsClient *firestore.Client, neon *NeonWeeklyPlanDraftRepository, userID string, dryRun bool, report *BackfillReport) {
	docs, err := fsClient.Collection("users").Doc(userID).Collection("weekly_plan_drafts").Documents(ctx).GetAll()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("weekly_plan_drafts user=%s: list: %v", userID, err))
		return
	}
	for _, doc := range docs {
		var dd weeklyPlanDraftDoc
		if err := doc.DataTo(&dd); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("weekly_plan_drafts user=%s doc=%s: decode: %v", userID, doc.Ref.ID, err))
			continue
		}
		var draft usecase.WeekDraft
		if err := json.Unmarshal([]byte(dd.DraftJSON), &draft); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("weekly_plan_drafts user=%s doc=%s: unmarshal: %v", userID, doc.Ref.ID, err))
			continue
		}
		draft.ID = doc.Ref.ID
		draft.UserID = userID
		report.WeeklyPlanDrafts++
		if dryRun {
			continue
		}
		if err := neon.SaveDraftLinked(ctx, &draft, doc.Ref.ID); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("weekly_plan_drafts user=%s doc=%s: write: %v", userID, doc.Ref.ID, err))
		}
	}
}

// -------------------- exercise pins --------------------

func backfillExercisePins(ctx context.Context, fsClient *firestore.Client, neon *NeonExercisePinRepository, userID string, dryRun bool, report *BackfillReport) {
	docs, err := fsClient.Collection("users").Doc(userID).Collection("exercise_pins").Documents(ctx).GetAll()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("exercise_pins user=%s: list: %v", userID, err))
		return
	}
	for _, doc := range docs {
		var pd exercisePinDoc
		if err := doc.DataTo(&pd); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("exercise_pins user=%s doc=%s: decode: %v", userID, doc.Ref.ID, err))
			continue
		}
		report.ExercisePins++
		if dryRun {
			continue
		}

		ids := make([]mesocycle.ExerciseID, len(pd.ExerciseIDs))
		for i, id := range pd.ExerciseIDs {
			ids[i] = mesocycle.ExerciseID(id)
		}
		pin := mesocycle.PinnedExerciseSet{
			ActivityType: activity.ID(pd.ActivityType),
			MuscleGroup:  activity.MuscleGroup(pd.MuscleGroup),
			ExerciseIDs:  ids,
			ActivatedOn:  pd.ActivatedOn,
			RotatesOn:    pd.RotatesOn,
		}
		if err := neon.Save(ctx, userID, pin); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("exercise_pins user=%s doc=%s: write: %v", userID, doc.Ref.ID, err))
		}
	}
}

// -------------------- daily check-ins --------------------

func backfillDailyCheckins(ctx context.Context, fsClient *firestore.Client, neon *NeonDailyCheckinRepository, userID string, dryRun bool, report *BackfillReport) {
	docs, err := fsClient.Collection("users").Doc(userID).Collection("daily_checkins").Documents(ctx).GetAll()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("daily_checkins user=%s: list: %v", userID, err))
		return
	}
	for _, doc := range docs {
		var d dailyCheckinDoc
		if err := doc.DataTo(&d); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("daily_checkins user=%s doc=%s: decode: %v", userID, doc.Ref.ID, err))
			continue
		}
		date, err := time.Parse("2006-01-02", doc.Ref.ID)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("daily_checkins user=%s doc=%s: bad date doc id: %v", userID, doc.Ref.ID, err))
			continue
		}
		report.DailyCheckins++
		if dryRun {
			continue
		}

		dc := &domain.DailyCheckin{
			UserID: userID,
			Date:   date,
			Answers: checkin.DailyCheckin{
				Sleep:  checkin.SleepAnswer(d.Sleep),
				Body:   checkin.BodyAnswer(d.Body),
				Demand: checkin.DemandAnswer(d.Demand),
				Need:   checkin.NeedAnswer(d.Need),
			},
			Readiness: checkin.ReadinessDimensions{
				RecoveryCapacity: checkin.RecoveryCapacity(d.Recovery),
				LifeBandwidth:    checkin.LifeBandwidth(d.Bandwidth),
				BuildReadiness:   checkin.BuildReadiness(d.Build),
			},
			SubmittedAt: d.SubmittedAt,
		}
		if err := neon.Upsert(ctx, dc); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("daily_checkins user=%s doc=%s: write: %v", userID, doc.Ref.ID, err))
		}
	}
}

// -------------------- session logs --------------------

func backfillSessionLogs(ctx context.Context, fsClient *firestore.Client, neon *NeonSessionLogRepository, userID string, dryRun bool, report *BackfillReport) {
	docs, err := fsClient.Collection("users").Doc(userID).Collection("session_logs").Documents(ctx).GetAll()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("session_logs user=%s: list: %v", userID, err))
		return
	}
	for _, doc := range docs {
		var d sessionLogDoc
		if err := doc.DataTo(&d); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("session_logs user=%s doc=%s: decode: %v", userID, doc.Ref.ID, err))
			continue
		}
		date, err := time.Parse("2006-01-02", doc.Ref.ID)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("session_logs user=%s doc=%s: bad date doc id: %v", userID, doc.Ref.ID, err))
			continue
		}
		report.SessionLogs++
		if dryRun {
			continue
		}

		sets := make([]domain.SetLog, len(d.Sets))
		for i, s := range d.Sets {
			sets[i] = domain.SetLog{
				ExerciseID: s.ExerciseID, ExerciseName: s.ExerciseName, SetNumber: s.SetNumber,
				WeightKg: s.WeightKg, Reps: s.Reps, LoggedAt: s.LoggedAt,
			}
		}
		log := &domain.SessionLog{
			UserID:       userID,
			Date:         date,
			ActivityType: d.ActivityType,
			Status:       domain.SessionLogStatus(d.Status),
			StartedAt:    d.StartedAt,
			EndedAt:      d.EndedAt,
			Sets:         sets,
			Feedback:     domain.SessionFeedback(d.Feedback),
			UpdatedAt:    d.UpdatedAt,
		}
		if err := neon.Upsert(ctx, log); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("session_logs user=%s doc=%s: write: %v", userID, doc.Ref.ID, err))
		}
	}
}

// -------------------- nutrition plan (single current doc) --------------------

func backfillNutritionPlan(ctx context.Context, fsClient *firestore.Client, neon *NeonNutritionPlanRepository, userID string, dryRun bool, report *BackfillReport) {
	fs := NewFirestoreNutritionPlanRepository(fsClient)
	plan, err := fs.GetByUserID(ctx, userID)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("nutrition_plan user=%s: get: %v", userID, err))
		return
	}
	if plan == nil {
		return
	}
	report.NutritionPlans++
	if dryRun {
		return
	}
	if err := neon.Save(ctx, userID, plan.Plan); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("nutrition_plan user=%s: write: %v", userID, err))
	}
}

// -------------------- recovery actions --------------------

func backfillRecoveryActions(ctx context.Context, fsClient *firestore.Client, neon *NeonRecoveryActionRepository, userID string, dryRun bool, report *BackfillReport) {
	docs, err := fsClient.Collection("users").Doc(userID).Collection("recovery_actions").Documents(ctx).GetAll()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("recovery_actions user=%s: list: %v", userID, err))
		return
	}
	for _, doc := range docs {
		var d recoveryActionDoc
		if err := doc.DataTo(&d); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("recovery_actions user=%s doc=%s: decode: %v", userID, doc.Ref.ID, err))
			continue
		}
		date, err := time.Parse("2006-01-02", doc.Ref.ID)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("recovery_actions user=%s doc=%s: bad date doc id: %v", userID, doc.Ref.ID, err))
			continue
		}
		report.RecoveryActions++
		if dryRun {
			continue
		}

		a := &domain.RecoveryAction{
			UserID:   userID,
			Date:     date,
			Kind:     domain.RecoveryActionKind(d.Kind),
			LoggedAt: d.LoggedAt,
		}
		if err := neon.Upsert(ctx, a); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("recovery_actions user=%s doc=%s: write: %v", userID, doc.Ref.ID, err))
		}
	}
}

// -------------------- consents --------------------

func backfillConsents(ctx context.Context, fsClient *firestore.Client, neon *NeonConsentRepository, userID string, dryRun bool, report *BackfillReport) {
	docs, err := fsClient.Collection("users").Doc(userID).Collection("consents").Documents(ctx).GetAll()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("consents user=%s: list: %v", userID, err))
		return
	}
	for _, doc := range docs {
		var d consentDoc
		if err := doc.DataTo(&d); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("consents user=%s doc=%s: decode: %v", userID, doc.Ref.ID, err))
			continue
		}
		report.Consents++
		if dryRun {
			continue
		}

		if err := neon.CreateHistorical(ctx, userID, domain.ConsentPurpose(d.Purpose), d.Granted, d.PolicyVersion, d.CreatedAt); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("consents user=%s doc=%s: write: %v", userID, doc.Ref.ID, err))
		}
	}
}
