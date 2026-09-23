package usecase

import (
	"context"
	"time"

	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/goal"
	weeklytarget "viv/internal/core/weekly_target"
)

// ============================================================================
// WeekDraft — the shape that flows through GenerateWeeklyPlanUsecase
// ============================================================================
//
// This is the new-system's equivalent of domain.WeekArrangement, but it is
// deliberately its own type rather than a reuse of that struct: the old
// WeekArrangement is keyed to the old rule engine's domain.Modality /
// domain.Intensity / domain.MuscleGroup types, and this pipeline (VIV-101
// through VIV-105) never mixes vocabularies with the old system.

// DayPlan is one day of a WeekDraft. Assignment is the zero value when
// IsRestDay is true.
type DayPlan struct {
	Date       time.Time
	Weekday    string // lowercase, e.g. "monday" — matches the convention already used for domain.Plan.TrainingCompleted keys
	IsRestDay  bool
	Assignment cascade.SlotAssignment

	// Warning is a UI-facing annotation set by VIV-111's warnings layer
	// (e.g. a joint-impact warning) — "" means no warning. A single
	// string, not a slice, so DayPlan (and WeekDraft.Days) stay
	// comparable with == — several existing tests rely on that. Purely
	// additive: nothing ever reads Warning to change Assignment.
	Warning string

	// Content is VIV-112's hydrated output — nil for rest days, or before
	// Layer 3 has run. A pointer (not embedded) keeps DayPlan cheap to
	// copy when Content isn't needed, and — like Warning above — doesn't
	// break DayPlan/[7]DayPlan's comparability with == (pointers are
	// comparable; only slices/maps aren't).
	Content *SelectedContent
}

// WeekDraft is a full 7-day plan, still editable, as produced by
// GenerateWeeklyPlanUsecase before a human ever sees it. "Draft" refers to
// its persisted Status, not to the completeness of the data — by the time
// it's saved, every stage (including the VIV-109..112 stub layers below)
// has already run over it once.
type WeekDraft struct {
	ID             string
	UserID         string
	GenerationDate time.Time
	StartDate      time.Time
	EndDate        time.Time
	Status         string // "draft" — matches domain.Plan.Status's convention of a plain status string
	GoalID         goal.ID
	Readiness      checkin.ReadinessDimensions
	Target         weeklytarget.WeeklyTarget
	Days           [7]DayPlan // fixed-size, mirroring domain.WeekArrangement's "always exactly 7 days" guarantee
	CreatedAt      time.Time

	// Notes caches VIV-113's weekly note per date already generated for
	// this draft — keyed by "2006-01-02" (the same date GetWeeklyNoteInput
	// is scoped to), value is the generated note text. See
	// GetWeeklyNoteUseCase and WeeklyPlanDraftRepository.SetNote. Cleared
	// by UpdateDaySlot whenever any day changes, since a stale note could
	// describe content that no longer matches the week.
	Notes map[string]string
}

// ============================================================================
// Downstream layer interfaces (design doc §9-§13)
// ============================================================================
//
// None of these have real implementations yet — VIV-109 (scheduling),
// VIV-110 (rule engine validation), VIV-111 (warnings/overrides), and
// VIV-112 (content selection) build them. GenerateWeeklyPlanUsecase depends
// only on these interfaces, wired to the Noop* stubs below for now, so each
// one can be swapped independently later without touching the orchestrator.

// SchedulingLayer assigns real calendar slots (day/time-of-day) to a
// WeekDraft's sessions. Design doc's "Layer 1". Real implementation: VIV-109.
//
// feedback is "" on a normal call. VIV-110's RuleEngineValidator passes a
// non-empty feedback string when it's calling this a second time because
// the first scheduling attempt violated a spacing rule after the fact —
// see internal/adapters/llm/openai/weekly_scheduler.go for how a real
// implementation threads this into its prompt.
type SchedulingLayer interface {
	Schedule(ctx context.Context, draft WeekDraft, feedback string) (WeekDraft, error)
}

// ValidationEngine runs the rule-engine validation pass over a scheduled
// WeekDraft. Real implementation: VIV-110.
type ValidationEngine interface {
	Validate(ctx context.Context, draft WeekDraft) (WeekDraft, error)
}

// WarningsOverridesLayer attaches user-facing warnings and applies any
// standing user overrides. Design doc's "Layer 2". Real implementation: VIV-111.
type WarningsOverridesLayer interface {
	Apply(ctx context.Context, draft WeekDraft) (WeekDraft, error)
}

// ContentSelectionLayer hydrates each session with real content (exercises,
// instructions, etc). Design doc's "Layer 3". Real implementation: VIV-112.
type ContentSelectionLayer interface {
	SelectContent(ctx context.Context, draft WeekDraft) (WeekDraft, error)
}

// WeeklyPlanDraftRepository persists a WeekDraft as an editable draft the
// user can view and modify. Mirrors PlanRepository's shape (Create sets the
// generated ID on the passed-in pointer) — see FirestoreWeeklyPlanDraftRepository.
type WeeklyPlanDraftRepository interface {
	SaveDraft(ctx context.Context, draft *WeekDraft) error

	// GetByDate returns the WeekDraft whose [StartDate, EndDate] span
	// contains date, or nil if no generated week covers it. VIV-107 (daily
	// adaptation) and UserEditSlotUsecase both load the whole week this
	// way — they still only ever touch the one day they're asked about.
	GetByDate(ctx context.Context, userID string, date time.Time) (*WeekDraft, error)

	// UpdateDaySlot persists a change to exactly one day (0-6) of an
	// already-saved draft, without rewriting the rest of the week. This is
	// what lets adaptation and manual edits guarantee they never touch any
	// other day's SlotAssignment. Also clears the draft's whole Notes
	// cache (see SetNote) — a day changing anywhere in the week can make
	// any already-cached note stale, since WeeklyNoteInput summarizes all
	// 7 days, not just the one requested.
	UpdateDaySlot(ctx context.Context, userID, draftID string, dayIndex int, day DayPlan) error

	// SetNote caches a generated weekly note (VIV-113) for one date of an
	// already-saved draft, keyed the same way as WeekDraft.Notes
	// ("2006-01-02"). Best-effort from the caller's point of view —
	// GetWeeklyNoteUseCase still returns the note it just generated even
	// if the cache write fails; a cache miss next time just costs another
	// LLM call, not a broken response.
	SetNote(ctx context.Context, userID, draftID, dateKey, note string) error
}

// ============================================================================
// Stub / no-op implementations
// ============================================================================
//
// Each one passes its input through completely unchanged. They exist so the
// orchestrator's shape is correct and testable today, ahead of VIV-109..112.

type NoopSchedulingLayer struct{}

func (NoopSchedulingLayer) Schedule(_ context.Context, draft WeekDraft, _ string) (WeekDraft, error) {
	return draft, nil
}

type NoopValidationEngine struct{}

func (NoopValidationEngine) Validate(_ context.Context, draft WeekDraft) (WeekDraft, error) {
	return draft, nil
}

type NoopWarningsOverridesLayer struct{}

func (NoopWarningsOverridesLayer) Apply(_ context.Context, draft WeekDraft) (WeekDraft, error) {
	return draft, nil
}

type NoopContentSelectionLayer struct{}

func (NoopContentSelectionLayer) SelectContent(_ context.Context, draft WeekDraft) (WeekDraft, error) {
	return draft, nil
}
