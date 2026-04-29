package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/recovery"
)

type GetRecoveryInput struct {
	UserID string
}

type GetRecoveryUseCase struct {
	users     UserRepository
	plans     PlanRepository
	cycle     CyclePhaseLookup
	bannerLib *recovery.BannerLibrary
	movesLib  *recovery.MovesContentLibrary
}

func NewGetRecoveryUseCase(
	users UserRepository,
	plans PlanRepository,
	cycle CyclePhaseLookup,
	bannerLib *recovery.BannerLibrary,
	movesLib *recovery.MovesContentLibrary,
) *GetRecoveryUseCase {
	return &GetRecoveryUseCase{
		users:     users,
		plans:     plans,
		cycle:     cycle,
		bannerLib: bannerLib,
		movesLib:  movesLib,
	}
}

func (uc *GetRecoveryUseCase) Execute(ctx context.Context, in GetRecoveryInput) (domain.RecoveryDay, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return domain.RecoveryDay{}, fmt.Errorf("user_id is required")
	}

	// Load user
	user, err := uc.users.GetByID(ctx, userID)
	if err != nil || user == nil {
		return domain.RecoveryDay{}, fmt.Errorf("loading user: %w", err)
	}

	// Get phase
	phase, err := uc.cycle.CurrentPhase(ctx, userID)
	if err != nil {
		return domain.RecoveryDay{}, fmt.Errorf("getting cycle phase: %w", err)
	}

	// Check if first week
	isFirstWeek := isUserFirstWeek(user)

	// Load current plan
	plan, err := uc.plans.GetLatest(ctx, userID)
	if err != nil || plan == nil {
		// No plan yet — show onboarding variant
		banner := recovery.SelectBanner(recovery.BannerInput{
			Phase:       phase,
			IsFirstWeek: true,
			Weekday:     time.Now().Weekday(),
		}, uc.bannerLib)

		return domain.RecoveryDay{
			Banner: banner,
			Moves:  nil,
		}, nil
	}

	// Parse training plan to determine today's and yesterday's status
	var trainingPlan domain.TrainingWeekPlan
	if err := json.Unmarshal(plan.TrainingJSON, &trainingPlan); err != nil {
		return domain.RecoveryDay{}, fmt.Errorf("parsing training plan: %w", err)
	}

	now := time.Now()
	todayIdx := weekdayToIndex(now.Weekday())
	yesterdayIdx := (todayIdx + 6) % 7 // wrap around

	// Is today a training day?
	isTrainingDay := !trainingPlan.Days[todayIdx].IsRestDay &&
		trainingPlan.Days[todayIdx].Session != nil

	// Did she train yesterday?
	trainedYesterday := false
	var yesterdayModality domain.Modality

	// Check training_completed first (did she actually do it?)
	if plan.TrainingCompleted != nil {
		yesterdayName := indexToWeekdayName(yesterdayIdx)
		if plan.TrainingCompleted[yesterdayName] {
			trainedYesterday = true
			if trainingPlan.Days[yesterdayIdx].Session != nil {
				yesterdayModality = trainingPlan.Days[yesterdayIdx].Session.Modality
			}
		}
	}

	// Count consecutive rest days
	consecutiveRest := countConsecutiveRestDays(plan.TrainingCompleted, todayIdx)

	input := recovery.BannerInput{
		Phase:               phase,
		IsTrainingDay:       isTrainingDay,
		TrainedYesterday:    trainedYesterday,
		YesterdayModality:   yesterdayModality,
		ConsecutiveRestDays: consecutiveRest,
		IsFirstWeek:         isFirstWeek,
		Weekday:             now.Weekday(),
	}

	banner := recovery.SelectBanner(input, uc.bannerLib)

	// ── Moves ────────────────────────────────────────────────────
	var moves []domain.RecoveryMove

	if isTrainingDay {
		// Training day — no recovery moves (Variant T is forward-looking)
		moves = nil
	} else if isFirstWeek {
		// First week — no moves yet
		moves = nil
	} else {
		// Rest day — select and hydrate moves
		moveIDs := recovery.SelectMoves(phase, yesterdayModality, consecutiveRest)

		// Try to get pre-generated content
		sessionTypeStr := string(yesterdayModality)
		preGenerated := uc.movesLib.GetMoves(string(phase), sessionTypeStr, consecutiveRest)

		if preGenerated != nil {
			// Use pre-generated content
			for _, m := range preGenerated {
				moves = append(moves, domain.RecoveryMove{
					MoveID: m.MoveID,
					Title:  m.Title,
					Timing: m.Timing,
					Detail: m.Detail,
					Impact: m.Impact,
				})
			}
		} else {
			// Fallback — use move library definitions without language
			for _, id := range moveIDs {
				if def, ok := recovery.GetMove(id); ok {
					moves = append(moves, domain.RecoveryMove{
						MoveID: def.ID,
						Title:  def.Name,
						Timing: "Today",
						Detail: "",
						Impact: def.Impact,
					})
				}
			}
		}
	}

	return domain.RecoveryDay{
		Banner: banner,
		Moves:  moves,
	}, nil
}

func isUserFirstWeek(user *domain.User) bool {
	if user.CreatedAt.IsZero() {
		return true
	}
	return time.Since(user.CreatedAt) < 7*24*time.Hour
}

func weekdayToIndex(wd time.Weekday) int {
	// Our array is Mon=0 through Sun=6
	switch wd {
	case time.Monday:
		return 0
	case time.Tuesday:
		return 1
	case time.Wednesday:
		return 2
	case time.Thursday:
		return 3
	case time.Friday:
		return 4
	case time.Saturday:
		return 5
	case time.Sunday:
		return 6
	default:
		return 0
	}
}

func indexToWeekdayName(idx int) string {
	names := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	if idx < 0 || idx > 6 {
		return "monday"
	}
	return names[idx]
}

func countConsecutiveRestDays(completed map[string]bool, todayIdx int) int {
	count := 0

	// Count backwards from today (inclusive if today is rest)
	for i := todayIdx; i >= 0; i-- {
		dayName := indexToWeekdayName(i)

		// If she completed training this day, break
		if completed != nil && completed[dayName] {
			break
		}

		// If it's a scheduled training day she hasn't completed, still counts as rest
		count++
	}

	if count == 0 {
		count = 1
	}

	return count
}
