package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	fb "firebase.google.com/go"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"google.golang.org/api/option"

	"viv/internal/adapters/apple"
	"viv/internal/adapters/email"
	httpadapter "viv/internal/adapters/http"
	"viv/internal/adapters/llm/openai"
	"viv/internal/adapters/mealgen"
	"viv/internal/adapters/repository"
	"viv/internal/adapters/runner"
	"viv/internal/config"
	corecontent "viv/internal/core/content"
	"viv/internal/core/mesocycle"
	corenutrition "viv/internal/core/nutrition"
	"viv/internal/core/rules"
	rulestraining "viv/internal/core/rules/training"
	coretraining "viv/internal/core/training"
	"viv/internal/core/usecase"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"

	_ "time/tzdata"

	cron "github.com/robfig/cron/v3"
)

func main() {

	// ====== Sentry initialization ======
	// TracesSampleRate defaults to 0.2 (20% of requests get a performance
	// trace) rather than 0, so the Performance tab in Sentry actually has
	// data without needing a code change — tune via SENTRY_TRACES_SAMPLE_RATE.
	// Release comes from SENTRY_RELEASE (e.g. the deploy's git SHA) so
	// Sentry can tell "did this error start after a specific deploy"
	// instead of lumping every version together; left empty if unset,
	// which Sentry tolerates (events just go untagged by release).
	tracesSampleRate := 0.2
	if v := os.Getenv("SENTRY_TRACES_SAMPLE_RATE"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			tracesSampleRate = parsed
		} else {
			log.Printf("[startup] invalid SENTRY_TRACES_SAMPLE_RATE %q, using default %.2f", v, tracesSampleRate)
		}
	}
	err_sentry := sentry.Init(sentry.ClientOptions{
		Dsn:              os.Getenv("SENTRY_DSN"),
		Environment:      os.Getenv("APP_ENV"),
		Release:          os.Getenv("SENTRY_RELEASE"),
		AttachStacktrace: true,
		TracesSampleRate: tracesSampleRate,
	})
	if err_sentry != nil {
		log.Printf("sentry init failed: %v", err_sentry)
	}
	// log.Fatal below calls os.Exit, which skips every deferred call —
	// including this one — so a startup crash needs its own flush (see
	// the fatal() helper at the bottom of this file) rather than relying
	// on this defer, which only covers a clean return from main().
	defer sentry.Flush(2 * time.Second)

	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic:         true,
		WaitForDelivery: true,
	})

	// ========= Config load =========
	cfg, err := config.Load()
	if err != nil {
		fatal("failed to load config: %v", err)
	}

	// ========= Context de app + graceful shutdown =========
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ========= Firebase App Initialization =========
	log.Printf("FIREBASE_PROJECT_ID=%q", cfg.FirebaseProjectID)
	log.Printf("GOOGLE_APPLICATION_CREDENTIALS=%q", cfg.FirebaseCredentialsFile)
	app, err := initFirebaseApp(ctx, cfg)
	if err != nil {
		fatal("failed to init firebase app: %v", err)
	}

	// Auth client
	authClient, err := app.Auth(ctx)
	if err != nil {
		fatal("failed to init firebase auth client: %v", err)
	}

	// ========= Repositories =========
	// Phase 4 of the Firestore→Neon cutover removed Firestore entirely —
	// Firebase is now used only for Auth (authClient above) and push
	// notifications (FCMRepository below, which rides on the same *fb.App
	// but never touches Firestore). See the Audit Map for the full
	// per-entity cutover history.

	// Neon is mandatory now, not an optional dual-write target. Phase 1 of
	// the Firestore→Neon cutover (checkins/device tokens/lifestyle
	// changes/weekly plan drafts/exercise pins/daily check-ins/session
	// logs/nutrition plans/recovery actions — see the Audit Map) made
	// those 9 entities Neon-only: there is no Firestore fallback for them
	// anymore, so a missing/unreachable Neon is fatal at startup instead
	// of the silent "Firestore-only mode" degradation this used to have.
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fatal("DATABASE_URL is required — Firestore-only mode was retired in Phase 1 of the Firestore→Neon cutover")
	}
	neonPool, err := repository.NewNeonPool(ctx, dbURL)
	if err != nil {
		fatal("neon: failed to connect: %v", err)
	}
	slog.Info("neon: connected")

	// Phase 1 cutover — Neon-only, no Firestore involved for these at all.
	var (
		checkinRepo         usecase.CheckinRepository         = repository.NewNeonCheckinRepository(neonPool)
		deviceTokenRepo     usecase.DeviceTokenRepository     = repository.NewNeonDeviceTokenRepository(neonPool)
		lifestyleRepo       usecase.LifestyleChangeRepository = repository.NewNeonLifestyleChangeRepository(neonPool)
		weeklyPlanDraftRepo usecase.WeeklyPlanDraftRepository = repository.NewNeonWeeklyPlanDraftRepository(neonPool)
		exercisePinRepo     usecase.ExercisePinRepository     = repository.NewNeonExercisePinRepository(neonPool)
		dailyCheckinRepo    usecase.DailyCheckinRepository    = repository.NewNeonDailyCheckinRepository(neonPool)
		sessionLogRepo      usecase.SessionLogRepository      = repository.NewNeonSessionLogRepository(neonPool)
		nutritionPlanRepo   usecase.NutritionPlanRepository   = repository.NewNeonNutritionPlanRepository(neonPool)
		recoveryActionRepo  usecase.RecoveryActionRepository  = repository.NewNeonRecoveryActionRepository(neonPool)
	)

	// Phase 2 cutover — User, legacy Plan, and PlanJobs are Neon-only now
	// too (data parity verified via a fresh error-free backfill run).
	// neonUserRepo is kept as its concrete type alongside the userRepo
	// interface variable because it also satisfies PendingDeletionLister/
	// InactivityLister (used by the two crons below) — usecase.UserRepository
	// itself doesn't expose those methods.
	neonUserRepo := repository.NewNeonUserRepository(neonPool)
	userRepo := usecase.UserRepository(neonUserRepo)
	planRepo := usecase.PlanRepository(repository.NewNeonPlanRepository(neonPool))
	planJobsRepo := usecase.PlanJobsRepository(repository.NewNeonPlanJobsRepository(neonPool))

	// Phase 4 cutover — Neon-only, no Firestore involved at all anymore.
	consentRepo := usecase.ConsentStore(repository.NewNeonConsentRepository(neonPool))
	accountAnonymizer := usecase.AccountAnonymizer(repository.NewNeonAccountAnonymizer(neonPool))
	ineligibilityEventRecorder := usecase.IneligibilityEventRecorder(repository.NewNeonIneligibilityEventRecorder(neonPool))

	fcmRepo := repository.NewFCMRepository(app)

	if cfg.OpenAIAPIKey == "" {
		fatal("OPENAI_API_KEY is not set")
	}

	oaClient := openai.NewOpenAIClient(cfg.OpenAIAPIKey, "gpt-4.1-mini")

	// ========= Training Pipeline =========
	trainingLib, err := coretraining.LoadLibrary("internal/content/training")
	if err != nil {
		fatal("failed to load training library: %v", err)
	}
	log.Printf("training library loaded: %d sessions", trainingLib.SessionCount())

	trainingEngine := rules.NewEngine(rulestraining.NewTrainingRuleSet())
	log.Printf("training rule engine initialized: %d rules", trainingEngine.RuleCount())
	trainingStructureGen := openai.NewTrainingStructureGenerator(oaClient)
	generateTrainingUC := usecase.NewGenerateTrainingPlanUsecase(
		trainingStructureGen,
		trainingEngine,
		trainingLib,
	)

	// ========= Nutrition Pipeline =========
	// Deterministic ingredient/template/solver library, replacing the live
	// LLM call (openai.NewMealContentGenerator) — see design discussion:
	// a curated ingredient library + a per-user macro solve is faster,
	// cheaper, and doesn't need a schema-conformance retry loop the way
	// free-form LLM generation does.
	nutritionIngredients, err := corenutrition.LoadIngredientLibrary("internal/content/nutrition")
	if err != nil {
		fatal("failed to load nutrition ingredient library: %v", err)
	}
	log.Printf("nutrition ingredient library loaded: %d ingredients", nutritionIngredients.Count())

	nutritionTemplates, err := corenutrition.LoadMealTemplateLibrary("internal/content/nutrition")
	if err != nil {
		fatal("failed to load nutrition template library: %v", err)
	}
	log.Printf("nutrition template library loaded: %d templates", nutritionTemplates.Count())

	mealGen := mealgen.New(nutritionIngredients, nutritionTemplates)
	generateNutritionUC := usecase.NewGenerateNutritionPlanUsecase(mealGen)

	// Copy polish: writes appetizing Name/Summary for already-solved plates
	// via LLM, in the background, AFTER the plan is saved and the job marked
	// done — never on the request path. Cache is process-local for now; the
	// bounded ingredient/template space means hit rate climbs fast anyway.
	copyGen := openai.NewCopyGenerator(oaClient)
	copyCache := mealgen.NewInMemoryCopyCache()
	copyEnricher := mealgen.NewAsyncCopyEnricher(copyGen, copyCache, planRepo, consentRepo)
	nutritionPlanCopyEnricher := mealgen.NewAsyncNutritionPlanCopyEnricher(copyGen, copyCache, nutritionPlanRepo, consentRepo)

	// ========= Usecases =========
	createCheckinUC := usecase.NewCreateCheckinUseCase(checkinRepo, userRepo, "v1")
	latestCheckinUC := usecase.NewGetLatestCheckinUseCase(checkinRepo)
	statusCheckinUC := usecase.NewGetCheckinStatusUseCase(checkinRepo, userRepo)
	cyclePhaseLookup := usecase.NewCyclePhaseAdapter(userRepo)

	reportLifestyleUC := usecase.NewReportLifestyleChangeUseCase(lifestyleRepo, userRepo)
	listLifestyleUC := usecase.NewListLifestyleChangesUseCase(lifestyleRepo)

	getMeUC := usecase.NewGetCurrentUserUseCase(userRepo)
	getCurrentPlanUC := usecase.NewGetCurrentPlanUseCase(userRepo, planRepo)

	resumeTrainingUC := usecase.NewResumeTrainingUseCase(userRepo)
	getByIDUC := usecase.NewGetPlanByIDUseCase(userRepo, planRepo)
	getByWeekStartUC := usecase.NewGetPlanByWeekStartUseCase(planRepo)

	statusUC := usecase.NewGetPlanGenerationStatusUseCase(planJobsRepo)
	completeDayUC := usecase.NewCompleteTrainingDayUseCase(planRepo)

	trainingRunner := runner.NewLocalTrainingPlanRunner(planJobsRepo, generateTrainingUC, generateNutritionUC, userRepo, checkinRepo, cyclePhaseLookup, planRepo, copyEnricher, 3*time.Minute)

	startTrainingUC := usecase.NewStartPlanGenerationUseCase(planJobsRepo, trainingRunner)
	statusTrainingUC := usecase.NewGetPlanGenerationStatusUseCase(planJobsRepo)
	saveArrangementUC := usecase.NewSaveTrainingArrangementUseCase(planRepo, userRepo, checkinRepo, trainingLib)

	// ========= New Weekly Plan Pipeline (VIV-106..113) =========
	// Deliberately isolated from the old Training Pipeline above — its own
	// content directory, its own taxonomy, its own async job flow, reusing
	// only what's genuinely shared (oaClient, cyclePhaseLookup,
	// planJobsRepo/PlanJob's queued/running/done/failed shape).
	weeklySessionLib, err := corecontent.LoadSessionLibrary("internal/content/training_v2/sessions")
	if err != nil {
		fatal("failed to load weekly-plan session library: %v", err)
	}
	weeklyExerciseLib, err := corecontent.LoadExerciseLibrary("internal/content/training_v2/exercises")
	if err != nil {
		fatal("failed to load weekly-plan exercise library: %v", err)
	}
	weeklyMesocycleWarmupCooldownLib, err := corecontent.LoadMesocycleWarmupCooldownLibrary("internal/content/training_v2/mesocycle_warmup_cooldown")
	if err != nil {
		fatal("failed to load mesocycle warmup/cooldown library: %v", err)
	}
	log.Printf("weekly-plan content loaded: session library + exercise library + mesocycle warmup/cooldown library")

	// StubExerciseSetSelector: real curated exercise-set selection logic
	// (VIV-108) doesn't exist yet — this just offers everything the
	// exercise library has for a muscle group. Swap when that lands.
	exercisePinService := usecase.NewExercisePinService(exercisePinRepo, mesocycle.StubExerciseSetSelector{}, weeklyExerciseLib)

	weeklyScheduler := openai.NewWeeklyScheduler(oaClient)                                                                                                // VIV-109, real LLM-backed Layer 1
	ruleEngineValidator := usecase.NewRuleEngineValidator(weeklyScheduler)                                                                                // VIV-110, retries Layer 1 once on a spacing violation
	jointImpactWarnings := usecase.NewJointImpactWarningsLayer()                                                                                          // VIV-111
	weeklyContentSelector := usecase.NewSessionContentSelector(weeklySessionLib, weeklyExerciseLib, weeklyMesocycleWarmupCooldownLib, exercisePinService) // VIV-112

	generateWeeklyPlanUC := usecase.NewGenerateWeeklyPlanUsecase(
		cyclePhaseLookup,
		weeklyScheduler,
		ruleEngineValidator,
		jointImpactWarnings,
		weeklyContentSelector,
		weeklyPlanDraftRepo,
	)

	weeklyPlanRunner := runner.NewLocalWeeklyPlanRunner(planJobsRepo, generateWeeklyPlanUC, 3*time.Minute)
	startWeeklyPlanUC := usecase.NewStartWeeklyPlanGenerationUseCase(planJobsRepo, weeklyPlanRunner)
	statusWeeklyPlanUC := usecase.NewGetPlanGenerationStatusUseCase(planJobsRepo)
	currentWeeklyPlanUC := usecase.NewGetCurrentWeeklyPlanUseCase(weeklyPlanDraftRepo)
	weeklyPlanDayUC := usecase.NewGetWeeklyPlanDayUseCase(weeklyPlanDraftRepo, sessionLogRepo)

	// Weekly note (VIV-113) — real LLM call, small plain-value input only
	// (see weekly_note.go's doc comments), wired here so it's finally
	// reachable via GET /training/weekly-plan/note.
	weeklyNoteGen := openai.NewWeeklyNoteGenerator(oaClient)
	weeklyNoteUC := usecase.NewWeeklyNoteUsecase(weeklyNoteGen)
	getWeeklyNoteUC := usecase.NewGetWeeklyNoteUseCase(weeklyPlanDraftRepo, weeklyNoteUC, consentRepo)

	// Manual day-slot edit (design doc §9) — wired here since it needs
	// weeklyContentSelector (VIV-112) to re-hydrate the edited day's
	// content, same collaborator generateWeeklyPlanUC above already uses.
	editDaySlotUC := usecase.NewUserEditSlotUsecase(weeklyPlanDraftRepo, weeklyContentSelector)

	// Real-time set-by-set session logging (Loggable/mesocycle-pinned
	// days only, i.e. Strength today) — the write side of the session-
	// detail screen above.
	startSessionUC := usecase.NewStartSessionUseCase(weeklyPlanDraftRepo, sessionLogRepo)
	logSetUC := usecase.NewLogSetUseCase(weeklyPlanDraftRepo, sessionLogRepo)
	completeSessionUC := usecase.NewCompleteSessionUseCase(weeklyPlanDraftRepo, sessionLogRepo)

	// Onboarding completion queues the user's first weekly-plan generation
	// right after persisting — async, via the same planJobsRepo/
	// weeklyPlanRunner pair POST /training/weekly-plan/generate uses, so
	// POST /onboarding returns without waiting on the LLM scheduling call.
	// Built here, after weeklyPlanRunner exists, since it needs it as a
	// collaborator.
	onboardingUC := usecase.NewCompleteOnboardingUseCase(userRepo, planJobsRepo, weeklyPlanRunner, consentRepo)

	// Daily check-in (VIV-103/106/107): scores the day's answers, then
	// either generates a fresh week (no plan covers today yet) or adapts
	// today's slot in an already-generated one.
	adaptDailySlotUC := usecase.NewAdaptDailySlotUsecase(cyclePhaseLookup, weeklyPlanDraftRepo)
	resyncNutritionPlanUC := usecase.NewResyncNutritionPlanUseCase(nutritionPlanRepo, userRepo)
	submitDailyCheckinUC := usecase.NewSubmitDailyCheckinUseCase(dailyCheckinRepo, weeklyPlanDraftRepo, userRepo, generateWeeklyPlanUC, adaptDailySlotUC, resyncNutritionPlanUC)

	// Always-available period logging — reuses the same cycle recalibration
	// as onboarding/check-in's cycle_start, but without the weekly check-in
	// gate (Mom Test P0: logging must work the day it happens, not only
	// Sunday). Drafts/generateWeeklyPlanUC/resyncNutritionPlanUC let a
	// report that lands off-prediction rebuild the current week around the
	// real anchor, same generator submitDailyCheckinUC's rollover uses.
	logPeriodStartUC := usecase.NewLogPeriodStartUseCase(userRepo, weeklyPlanDraftRepo, generateWeeklyPlanUC, resyncNutritionPlanUC)

	// PATCH /me needs generateNutritionUC (sync macro/meal recompute on
	// weight/height changes), startTrainingUC (async full regen on cycle
	// changes), and copyEnricher (so a weight/height-triggered nutrition
	// recompute gets its meal copy polished too, same as a fresh plan) —
	// built after all three exist.
	updateProfileUC := usecase.NewUpdateProfileUseCase(userRepo, planRepo, checkinRepo, cyclePhaseLookup, generateNutritionUC, startTrainingUC, copyEnricher)

	nutritionUC := usecase.NewGetNutritionPlanUseCase(userRepo, planRepo, checkinRepo, cyclePhaseLookup, nutritionPlanRepo)
	mealSelectionUC := usecase.NewSaveMealSelectionUseCase(planRepo)
	saveNutritionMealSelectionUC := usecase.NewSaveNutritionMealSelectionUseCase(nutritionPlanRepo)

	// New pipeline: nutrition is generated on demand once the user opts
	// into the nutrition module (see submit_nutrition_onboarding.go), not
	// bundled with training onboarding.
	submitNutritionOnboardingUC := usecase.NewSubmitNutritionOnboardingUseCase(userRepo, weeklyPlanDraftRepo, cyclePhaseLookup, generateNutritionUC, nutritionPlanRepo, nutritionPlanCopyEnricher)

	// Session-driven recovery card (VIV Recovery Engine Decision Table
	// spec). Replaces the old cycle-phase banner entirely — the old
	// GetRecoveryUseCase/domain.Plan-based /recovery/today was removed
	// because it silently faked an "onboarding" banner forever for any
	// new-pipeline user (who never has a domain.Plan). Reuses
	// weeklyContentSelector (VIV-112) to re-hydrate content for whichever
	// day a high-cost reschedule moves.
	getRecoveryCardUC := usecase.NewGetRecoveryCardUseCase(weeklyPlanDraftRepo, dailyCheckinRepo, userRepo, recoveryActionRepo, weeklyContentSelector)
	saveRecoveryActionUC := usecase.NewSaveRecoveryActionUseCase(recoveryActionRepo)

	// Home screen redesign — single aggregated read, reuses getRecoveryCardUC
	// as-is rather than re-threading weeklyContentSelector into a second place.
	getHomeTimelineUC := usecase.NewGetHomeTimelineUseCase(userRepo, weeklyPlanDraftRepo, dailyCheckinRepo, sessionLogRepo, nutritionPlanRepo, getRecoveryCardUC)

	phaseFeedbackUC := usecase.NewSavePhaseFeedbackUseCase(planRepo)

	registerDeviceTokenUC := usecase.NewRegisterDeviceTokenUseCase(deviceTokenRepo)
	sundayCheckinUC := usecase.NewSendSundayCheckinUseCase(deviceTokenRepo, fcmRepo)
	trainingReminderUC := usecase.NewSendTrainingReminderUseCase(deviceTokenRepo, fcmRepo, planRepo)

	// App Store account-deletion requirement (guideline 5.1.1v) — see
	// usecase.DeleteAccountUseCase's doc comment for the deletion ordering.
	authAccountDeleter := repository.NewFirebaseAuthAccountDeleter(authClient)

	// Sign in with Apple: token exchange (POST /me/apple-credentials) and
	// revocation (part of account deletion) — both the same App Store
	// requirement, both served by one apple.Client. All four APPLE_* env
	// vars must be set together; a partial set is treated the same as
	// none (both disabled, logged once here) rather than failing in a
	// half-configured way. Left as nil interfaces (not a nil *apple.Client
	// wrapped in one) when unconfigured, so the nil-checks below and in
	// DeleteAccountUseCase/ExchangeAppleAuthorizationCodeUseCase work.
	var (
		appleRevoker   usecase.AppleTokenRevoker
		appleExchanger usecase.AppleCodeExchanger
	)
	switch {
	case cfg.AppleTeamID == "" && cfg.AppleKeyID == "" && cfg.AppleClientID == "" && cfg.ApplePrivateKey == "":
		log.Println("[startup] APPLE_* credentials not set — Sign in with Apple token exchange/revocation is disabled")
	case cfg.AppleTeamID == "" || cfg.AppleKeyID == "" || cfg.AppleClientID == "" || cfg.ApplePrivateKey == "":
		log.Println("[startup] APPLE_* credentials only partially set — Sign in with Apple token exchange/revocation is disabled")
	default:
		appleClient, err := apple.NewClient(cfg.AppleTeamID, cfg.AppleKeyID, cfg.AppleClientID, cfg.ApplePrivateKey)
		if err != nil {
			log.Printf("[startup] failed to configure Apple client, disabling it: %v", err)
		} else {
			appleRevoker = appleClient
			appleExchanger = appleClient
		}
	}

	deleteAccountUC := usecase.NewDeleteAccountUseCase(accountAnonymizer, authAccountDeleter, userRepo, appleRevoker)
	exchangeAppleCodeUC := usecase.NewExchangeAppleAuthorizationCodeUseCase(userRepo, appleExchanger)

	// DELETE /me's 30-day grace period — see ScheduleAccountDeletionUseCase/
	// RunPendingAccountDeletionsUseCase's doc comments. ListPendingDeletions
	// is Neon-only now (Phase 2/3 of the Firestore→Neon cutover), hence
	// neonUserRepo rather than the userRepo interface variable.
	scheduleDeletionUC := usecase.NewScheduleAccountDeletionUseCase(userRepo)
	cancelDeletionUC := usecase.NewCancelAccountDeletionUseCase(userRepo)
	runPendingDeletionsUC := usecase.NewRunPendingAccountDeletionsUseCase(neonUserRepo, deleteAccountUC)
	recordActivityUC := usecase.NewRecordUserActivityUseCase(userRepo)

	// 2-year-inactivity sweep — "are you still there?" email via Brevo,
	// then the same 30-day grace period as a user-requested deletion (see
	// RunInactivitySweepUseCase). BrevoAPIKey empty just means every send
	// fails, is logged, and gets retried next week — it never silently
	// skips the notice/deletion flow.
	if cfg.BrevoAPIKey == "" {
		log.Println("[startup] BREVO_API_KEY not set — inactivity re-engagement emails will fail until it's configured")
	}
	reEngagementSender := email.NewBrevoReEngagementEmailSender(cfg.BrevoAPIKey, cfg.BrevoInactivityTemplateID)
	userContactLookup := repository.NewFirebaseUserContactLookup(authClient)
	runInactivitySweepUC := usecase.NewRunInactivitySweepUseCase(neonUserRepo, userContactLookup, reEngagementSender, userRepo)

	exportUserDataUC := usecase.NewExportUserDataUseCase(userRepo, consentRepo, lifestyleRepo, dailyCheckinRepo, sessionLogRepo, recoveryActionRepo, nutritionPlanRepo, weeklyPlanDraftRepo, planRepo)

	// Pre-onboarding eligibility gate ("Which describes you right now?") —
	// deletes the account outright, not through the 30-day grace period
	// (see ReportIneligibilityUseCase's doc comment).
	reportIneligibilityUC := usecase.NewReportIneligibilityUseCase(ineligibilityEventRecorder, deleteAccountUC)

	getConsentsUC := usecase.NewGetConsentsUseCase(consentRepo)
	setConsentUC := usecase.NewSetConsentUseCase(consentRepo, deleteAccountUC)

	// ========= Handlers =========
	onboardingHandler := httpadapter.NewOnboardingHandler(onboardingUC)
	checkinHandler := httpadapter.NewCheckinHandler(createCheckinUC, latestCheckinUC, statusCheckinUC)
	lifestyleHandler := httpadapter.NewLifestyleHandler(reportLifestyleUC, listLifestyleUC)
	meHandler := httpadapter.NewMeHandler(getMeUC, updateProfileUC, scheduleDeletionUC, cancelDeletionUC)
	homeHandler := httpadapter.NewHomeHandler(getHomeTimelineUC)
	consentHandler := httpadapter.NewConsentHandler(getConsentsUC, setConsentUC)
	exportHandler := httpadapter.NewExportHandler(exportUserDataUC)
	ineligibilityHandler := httpadapter.NewIneligibilityHandler(reportIneligibilityUC)
	appleHandler := httpadapter.NewAppleHandler(exchangeAppleCodeUC)
	plansHandler := httpadapter.NewPlansHandler(getCurrentPlanUC, getByIDUC, getByWeekStartUC, statusUC, phaseFeedbackUC)
	trainingHandler := httpadapter.NewTrainingHandler(startTrainingUC, statusTrainingUC, trainingEngine, resumeTrainingUC, completeDayUC, saveArrangementUC)
	weeklyPlanHandler := httpadapter.NewWeeklyPlanHandler(startWeeklyPlanUC, statusWeeklyPlanUC, currentWeeklyPlanUC, weeklyPlanDayUC, getWeeklyNoteUC, editDaySlotUC)
	sessionLogHandler := httpadapter.NewSessionLogHandler(startSessionUC, logSetUC, completeSessionUC)
	dailyCheckinHandler := httpadapter.NewDailyCheckinHandler(submitDailyCheckinUC)
	nutritionHandler := httpadapter.NewNutritionHandler(nutritionUC, mealSelectionUC, submitNutritionOnboardingUC, saveNutritionMealSelectionUC)
	recoveryCardHandler := httpadapter.NewRecoveryCardHandler(getRecoveryCardUC, saveRecoveryActionUC)
	deviceTokenHandler := httpadapter.NewDeviceTokenHandler(registerDeviceTokenUC)
	periodHandler := httpadapter.NewPeriodHandler(logPeriodStartUC)

	// ========= Router config =========
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(90 * time.Second))

	r.Get("/health", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// TEMPORARY — remove once Sentry delivery is confirmed. Forces one
	// real event through the whole pipe (hub from sentryhttp's context →
	// CaptureException → Flush) without needing to wait for real prod
	// traffic to fail on its own.
	r.Get("/debug/sentry-test", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.CaptureException(fmt.Errorf("sentry-test: manual delivery check"))
		} else {
			sentry.CaptureException(fmt.Errorf("sentry-test: manual delivery check (no hub in context)"))
		}
		sentry.Flush(2 * time.Second)
		w.WriteHeader(stdhttp.StatusOK)
		_, _ = w.Write([]byte(`{"status":"sent, check sentry in a few seconds"}`))
	})

	api := chi.NewRouter()

	api.Post("/onboarding", onboardingHandler.ServeHTTP)
	api.Post("/onboarding/ineligibility", ineligibilityHandler.Report)

	// VIV-103/106/107 daily check-in — distinct from the older /checkins
	// (plural) weekly check-in below.
	api.Post("/checkin", dailyCheckinHandler.Submit)

	api.Post("/checkins", checkinHandler.Create)
	api.Get("/checkins/latest", checkinHandler.Latest)
	api.Get("/checkins/status", checkinHandler.Status)

	api.Post("/lifestyle-changes", lifestyleHandler.Report)
	api.Get("/lifestyle-changes", lifestyleHandler.List)

	api.Get("/me", meHandler.GetMe)
	api.Patch("/me", meHandler.UpdateProfile)
	api.Delete("/me", meHandler.DeleteAccount)
	api.Post("/me/cancel-deletion", meHandler.CancelDeletion)
	api.Post("/me/apple-credentials", appleHandler.SetCredentials)

	api.Get("/me/consents", consentHandler.GetConsents)
	api.Post("/me/consents", consentHandler.SetConsent)

	api.Get("/me/export", exportHandler.Export)

	api.Get("/home/today", homeHandler.GetToday)

	api.Get("/plans/current", plansHandler.GetCurrent)
	api.Get("/plans/{id}", plansHandler.GetByID)
	api.Get("/plans/week/{week_start}", plansHandler.GetByWeekStart)
	api.Get("/plans/generate/status", plansHandler.GenerateStatus)

	api.Post("/training/resume", trainingHandler.Resume)
	api.Post("/training/generate", trainingHandler.Generate)
	api.Get("/training/generate/status", trainingHandler.GenerateStatus)
	api.Post("/training/validate-arrangement", trainingHandler.ValidateArrangement)
	api.Post("/training/complete-day", trainingHandler.CompleteDay)
	api.Post("/training/save-arrangement", trainingHandler.SaveArrangement)

	// New weekly-plan pipeline (VIV-106..113) — async job, same shape as
	// /training/generate above.
	api.Post("/training/weekly-plan/generate", weeklyPlanHandler.Generate)
	api.Get("/training/weekly-plan/generate/status", weeklyPlanHandler.GenerateStatus)
	api.Get("/training/weekly-plan/current", weeklyPlanHandler.CurrentWeek)
	api.Get("/training/weekly-plan/day", weeklyPlanHandler.Day)
	api.Patch("/training/weekly-plan/day", weeklyPlanHandler.EditDaySlot)
	api.Get("/training/weekly-plan/note", weeklyPlanHandler.Note)
	api.Post("/training/weekly-plan/day/start", sessionLogHandler.Start)
	api.Post("/training/weekly-plan/day/log-set", sessionLogHandler.LogSet)
	api.Post("/training/weekly-plan/day/complete", sessionLogHandler.Complete)

	api.Get("/nutrition/plan", nutritionHandler.GetPlan)
	api.Post("/nutrition/meal-selection", nutritionHandler.SaveMealSelection)
	api.Post("/nutrition/onboarding", nutritionHandler.SubmitOnboarding)
	api.Get("/recovery/card", recoveryCardHandler.GetCard)
	api.Post("/recovery/card/action", recoveryCardHandler.SaveAction)
	api.Post("/plans/phase-feedback", plansHandler.SavePhaseFeedback)

	api.Post("/users/me/device-token", deviceTokenHandler.Upsert)
	api.Post("/cycle/period-start", periodHandler.LogStart)

	chi.Walk(api, func(method string, route string, handler stdhttp.Handler, middlewares ...func(stdhttp.Handler) stdhttp.Handler) error {
		log.Printf("[api.route] %s %s", method, route)
		return nil
	})

	protected := chi.NewRouter()
	protected.Use(httpadapter.AppCheckMiddleware(cfg.FirebaseProjectNumber, cfg.AppCheckEnforce))
	protected.Use(httpadapter.FirebaseAuthMiddleware(authClient))
	protected.Use(httpadapter.EnsureUserMiddleware(userRepo, recordActivityUC))

	rpcHandler := httpadapter.NewRPCHandler(api)
	protected.Post("/rpc", rpcHandler.Handle)

	protected.Mount("/", api)

	r.Mount("/", protected)

	// ==================== Schedulers ==============================
	// cron.Recover (via WithChain) catches a panic inside any job below
	// and reports it through sentryCronLogger instead of crashing the
	// whole process — robfig/cron doesn't recover panics on its own, and
	// a panic in a goroutine with nothing above it to catch one takes
	// the entire server down, HTTP included.
	c := cron.New(cron.WithLocation(time.UTC), cron.WithChain(cron.Recover(sentryCronLogger{})))
	c.AddFunc("0 15 * * 0", func() {
		log.Println("[scheduler] running sunday checkin")
		if err := sundayCheckinUC.Execute(context.Background()); err != nil {
			log.Printf("[scheduler] error: %v", err)
			captureSchedulerError("sunday_checkin", err)
		}
	})

	// Training reminder
	c.AddFunc("0 * * * *", func() {
		log.Printf("[scheduler] training reminder tick - checking timezones")
		ctx := context.Background()
		tokensByTimezone, err := trainingReminderUC.DeviceToken.GetAllActiveByTimezone(ctx)
		if err != nil {
			log.Printf("[scheduler] error getting tokens: %v", err)
			captureSchedulerError("training_reminder", err)
			return
		}

		log.Printf("[scheduler] found %d timezones", len(tokensByTimezone))

		for timezone, tokens := range tokensByTimezone {
			loc, err := time.LoadLocation(timezone)
			if err != nil {
				log.Printf("[scheduler] invalid timezone %s: %v", timezone, err)
				captureSchedulerError("training_reminder", err)
				continue
			}
			localHour := time.Now().In(loc).Hour()
			log.Printf("[scheduler] timezone %s - local hour: %d", timezone, localHour)
			if localHour == 9 {
				trainingReminderUC.ExecuteForTimezone(ctx, timezone, tokens)
			}
		}
	})

	// Account-deletion sweep: finishes off every DELETE /me whose 30-day
	// grace period (ScheduleAccountDeletionUseCase) has elapsed. Once a
	// day is plenty — this isn't time-sensitive down to the hour.
	c.AddFunc("0 4 * * *", func() {
		log.Println("[scheduler] running account-deletion sweep")
		if err := runPendingDeletionsUC.Execute(context.Background()); err != nil {
			log.Printf("[scheduler] account-deletion sweep error: %v", err)
			captureSchedulerError("account_deletion_sweep", err)
		}
	})

	// Inactivity sweep: finds users with 2 years of no activity, emails
	// them via Brevo, and starts their 30-day notice window. Weekly, not
	// daily — this is a years-long threshold, there's no need to check it
	// every day.
	c.AddFunc("0 5 * * 1", func() {
		log.Println("[scheduler] running inactivity sweep")
		if err := runInactivitySweepUC.Execute(context.Background()); err != nil {
			log.Printf("[scheduler] inactivity sweep error: %v", err)
			captureSchedulerError("inactivity_sweep", err)
		}
	})

	c.Start()
	defer c.Stop()

	// ========= Levantar servidor HTTP =========
	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.HTTPPort
	}

	srv := &stdhttp.Server{
		Addr:         ":" + port,
		Handler:      sentryHandler.Handle(r),
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("VIV backend listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != stdhttp.ErrServerClosed {
			fatal("http server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	} else {
		log.Println("server stopped cleanly")
	}
}

// captureSchedulerError reports a cron job's own returned error to
// Sentry, tagged by job name — distinct from sentryCronLogger below,
// which only ever sees a panic, not a normal error return.
func captureSchedulerError(job string, err error) {
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("scheduler_job", job)
		sentry.CaptureException(err)
	})
}

// sentryCronLogger adapts cron.Recover's panic reporting to Sentry —
// without this, cron.DefaultLogger would just print the panic to
// stdout, exactly the kind of "silently stopped running" failure
// monitoring is supposed to catch.
type sentryCronLogger struct{}

func (sentryCronLogger) Info(msg string, keysAndValues ...any) {
	log.Printf("[scheduler] %s %v", msg, keysAndValues)
}

func (sentryCronLogger) Error(err error, msg string, keysAndValues ...any) {
	log.Printf("[scheduler] ERROR %s: %v %v", msg, err, keysAndValues)
	sentry.CaptureException(err)
}

// fatal reports msg to Sentry and flushes before exiting — log.Fatal's
// os.Exit skips every deferred call, so without this a startup crash
// (bad config, Neon/Firebase unreachable, a missing content library)
// would only ever show up in stdout logs, never in Sentry, making a
// crash-looping deploy invisible to the one tool meant to page on it.
func fatal(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	sentry.CaptureMessage(msg)
	sentry.Flush(2 * time.Second)
	os.Exit(1)
}

func initFirebaseApp(ctx context.Context, cfg *config.Config) (*fb.App, error) {
	opts := []option.ClientOption{}

	credsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON")

	if credsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(credsJSON)))
	} else if cfg.FirebaseCredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.FirebaseCredentialsFile))
	}

	app, err := fb.NewApp(ctx, &fb.Config{
		ProjectID: cfg.FirebaseProjectID,
	}, opts...)
	if err != nil {
		return nil, err
	}
	return app, nil
}
