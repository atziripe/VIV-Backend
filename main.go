package main

import (
	"context"
	"log"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	fb "firebase.google.com/go"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"google.golang.org/api/option"

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
	err_sentry := sentry.Init(sentry.ClientOptions{
		Dsn:         os.Getenv("SENTRY_DSN"),
		Environment: os.Getenv("APP_ENV"),
	})
	if err_sentry != nil {
		log.Printf("sentry init failed: %v", err_sentry)
	}

	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic:         true,
		WaitForDelivery: true,
	})

	// ========= Config load =========
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// ========= Context de app + graceful shutdown =========
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ========= Firebase App Initialization =========
	log.Printf("FIREBASE_PROJECT_ID=%q", cfg.FirebaseProjectID)
	log.Printf("GOOGLE_APPLICATION_CREDENTIALS=%q", cfg.FirebaseCredentialsFile)
	app, err := initFirebaseApp(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to init firebase app: %v", err)
	}

	// Auth client
	authClient, err := app.Auth(ctx)
	if err != nil {
		log.Fatalf("failed to init firebase auth client: %v", err)
	}

	// Firestore client
	fsClient, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalf("failed to init firestore client: %v", err)
	}
	defer fsClient.Close()

	// ========= Repositories =========
	// Firestore (primary — source of truth during migration)
	fsUserRepo := repository.NewFirestoreUserRepository(fsClient)
	fsCheckinRepo := repository.NewFirestoreCheckinRepository(fsClient)
	fsLifestyleRepo := repository.NewFirestoreLifestyleChangeRepository(fsClient)
	fsPlanRepo := repository.NewFirestorePlanRepository(fsClient)
	fsPlanJobsRepo := repository.NewFirestorePlanJobsRepository(fsClient)
	fsDeviceTokenRepo := repository.NewFirestoreDeviceTokenRepository(fsClient)

	// New weekly-plan pipeline (VIV-106..113) — Firestore-only, no Neon
	// counterpart yet, same as lifestyleRepo below.
	weeklyPlanDraftRepo := repository.NewFirestoreWeeklyPlanDraftRepository(fsClient)
	exercisePinRepo := repository.NewFirestoreExercisePinRepository(fsClient)
	dailyCheckinRepo := repository.NewFirestoreDailyCheckinRepository(fsClient)
	sessionLogRepo := repository.NewFirestoreSessionLogRepository(fsClient)
	nutritionPlanRepo := repository.NewFirestoreNutritionPlanRepository(fsClient)
	recoveryActionRepo := repository.NewFirestoreRecoveryActionRepository(fsClient)

	// Neon (secondary — dual-write target)
	// If DATABASE_URL is missing or Neon is unreachable, the app falls back to
	// Firestore-only mode and logs a warning. No crash, no downtime.
	var (
		userRepo        usecase.UserRepository        = fsUserRepo
		checkinRepo     usecase.CheckinRepository     = fsCheckinRepo
		planRepo        usecase.PlanRepository        = fsPlanRepo
		planJobsRepo    usecase.PlanJobsRepository    = fsPlanJobsRepo
		deviceTokenRepo usecase.DeviceTokenRepository = fsDeviceTokenRepo
	)

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		neonPool, err := repository.NewNeonPool(ctx, dbURL)
		if err != nil {
			slog.Error("neon: failed to connect, running Firestore-only", "err", err)
		} else {
			slog.Info("neon: connected — dual-write enabled")
			userRepo = repository.NewDualUserRepository(fsUserRepo, repository.NewNeonUserRepository(neonPool))
			checkinRepo = repository.NewDualCheckinRepository(fsCheckinRepo, repository.NewNeonCheckinRepository(neonPool))
			planRepo = repository.NewDualPlanRepository(fsPlanRepo, repository.NewNeonPlanRepository(neonPool))
			planJobsRepo = repository.NewDualPlanJobsRepository(fsPlanJobsRepo, repository.NewNeonPlanJobsRepository(neonPool))
			deviceTokenRepo = repository.NewDualDeviceTokenRepository(fsDeviceTokenRepo, repository.NewNeonDeviceTokenRepository(neonPool))
		}
	} else {
		slog.Warn("DATABASE_URL not set — running Firestore-only (no dual-write)")
	}

	// lifestyleRepo has no Neon implementation yet — stays Firestore-only
	lifestyleRepo := fsLifestyleRepo

	fcmRepo := repository.NewFCMRepository(app)

	if cfg.OpenAIAPIKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}

	oaClient := openai.NewOpenAIClient(cfg.OpenAIAPIKey, "gpt-4.1-mini")

	// ========= Training Pipeline =========
	trainingLib, err := coretraining.LoadLibrary("internal/content/training")
	if err != nil {
		log.Fatalf("failed to load training library: %v", err)
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
		log.Fatalf("failed to load nutrition ingredient library: %v", err)
	}
	log.Printf("nutrition ingredient library loaded: %d ingredients", nutritionIngredients.Count())

	nutritionTemplates, err := corenutrition.LoadMealTemplateLibrary("internal/content/nutrition")
	if err != nil {
		log.Fatalf("failed to load nutrition template library: %v", err)
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
	copyEnricher := mealgen.NewAsyncCopyEnricher(copyGen, copyCache, planRepo)
	nutritionPlanCopyEnricher := mealgen.NewAsyncNutritionPlanCopyEnricher(copyGen, copyCache, nutritionPlanRepo)

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
	// only what's genuinely shared (fsClient, oaClient, cyclePhaseLookup,
	// planJobsRepo/PlanJob's queued/running/done/failed shape).
	weeklySessionLib, err := corecontent.LoadSessionLibrary("internal/content/training_v2/sessions")
	if err != nil {
		log.Fatalf("failed to load weekly-plan session library: %v", err)
	}
	weeklyExerciseLib, err := corecontent.LoadExerciseLibrary("internal/content/training_v2/exercises")
	if err != nil {
		log.Fatalf("failed to load weekly-plan exercise library: %v", err)
	}
	weeklyMesocycleWarmupCooldownLib, err := corecontent.LoadMesocycleWarmupCooldownLibrary("internal/content/training_v2/mesocycle_warmup_cooldown")
	if err != nil {
		log.Fatalf("failed to load mesocycle warmup/cooldown library: %v", err)
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

	// Weekly note — real LLM call, small plain-value input only
	// (see weekly_note.go's doc comments), wired here so it's finally
	// reachable via GET /training/weekly-plan/note.
	weeklyNoteGen := openai.NewWeeklyNoteGenerator(oaClient)
	weeklyNoteUC := usecase.NewWeeklyNoteUsecase(weeklyNoteGen)
	getWeeklyNoteUC := usecase.NewGetWeeklyNoteUseCase(weeklyPlanDraftRepo, weeklyNoteUC)

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
	onboardingUC := usecase.NewCompleteOnboardingUseCase(userRepo, planJobsRepo, weeklyPlanRunner)

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

	phaseFeedbackUC := usecase.NewSavePhaseFeedbackUseCase(planRepo)

	registerDeviceTokenUC := usecase.NewRegisterDeviceTokenUseCase(deviceTokenRepo)
	sundayCheckinUC := usecase.NewSendSundayCheckinUseCase(deviceTokenRepo, fcmRepo)
	trainingReminderUC := usecase.NewSendTrainingReminderUseCase(deviceTokenRepo, fcmRepo, planRepo)

	// ========= Handlers =========
	onboardingHandler := httpadapter.NewOnboardingHandler(onboardingUC)
	checkinHandler := httpadapter.NewCheckinHandler(createCheckinUC, latestCheckinUC, statusCheckinUC)
	lifestyleHandler := httpadapter.NewLifestyleHandler(reportLifestyleUC, listLifestyleUC)
	meHandler := httpadapter.NewMeHandler(getMeUC, updateProfileUC)
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

	api := chi.NewRouter()

	api.Post("/onboarding", onboardingHandler.ServeHTTP)

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
	protected.Use(httpadapter.FirebaseAuthMiddleware(authClient))
	protected.Use(httpadapter.EnsureUserMiddleware(userRepo))

	rpcHandler := httpadapter.NewRPCHandler(api)
	protected.Post("/rpc", rpcHandler.Handle)

	protected.Mount("/", api)

	r.Mount("/", protected)

	// ==================== Schedulers ==============================
	c := cron.New(cron.WithLocation(time.UTC))
	c.AddFunc("0 15 * * 0", func() {
		log.Println("[scheduler] running sunday checkin")
		if err := sundayCheckinUC.Execute(context.Background()); err != nil {
			log.Printf("[scheduler] error: %v", err)
		}
	})

	// Training reminder
	c.AddFunc("0 * * * *", func() {
		log.Printf("[scheduler] training reminder tick - checking timezones")
		ctx := context.Background()
		tokensByTimezone, err := trainingReminderUC.DeviceToken.GetAllActiveByTimezone(ctx)
		if err != nil {
			log.Printf("[scheduler] error getting tokens: %v", err)
			return
		}

		log.Printf("[scheduler] found %d timezones", len(tokensByTimezone))

		for timezone, tokens := range tokensByTimezone {
			loc, err := time.LoadLocation(timezone)
			if err != nil {
				log.Printf("[scheduler] invalid timezone %s: %v", timezone, err)
				continue
			}
			localHour := time.Now().In(loc).Hour()
			log.Printf("[scheduler] timezone %s - local hour: %d", timezone, localHour)
			if localHour == 9 {
				trainingReminderUC.ExecuteForTimezone(ctx, timezone, tokens)
			}
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
			log.Fatalf("http server error: %v", err)
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
