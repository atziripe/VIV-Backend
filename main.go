package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
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
	corenutrition "viv/internal/core/nutrition"
	"viv/internal/core/recovery"
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

	// ========= Recovery Pipeline =========
	bannerLib, err := recovery.LoadBannerLibrary("internal/content/recovery")
	if err != nil {
		log.Printf("warning: banner library not loaded: %v", err)
	} else {
		log.Printf("recovery banner library loaded: %d entries", bannerLib.EntryCount())
	}

	movesContentLib, err := recovery.LoadMovesContentLibrary("internal/content/recovery")
	if err != nil {
		log.Printf("warning: moves content library not loaded: %v", err)
	} else {
		log.Printf("recovery moves library loaded: %d entries", movesContentLib.EntryCount())
	}

	// ========= Usecases =========
	onboardingUC := usecase.NewCompleteOnboardingUseCase(userRepo)
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

	// PATCH /me needs generateNutritionUC (sync macro/meal recompute on
	// weight/height changes) and startTrainingUC (async full regen on cycle
	// changes) — built after both exist.
	updateProfileUC := usecase.NewUpdateProfileUseCase(userRepo, planRepo, checkinRepo, cyclePhaseLookup, generateNutritionUC, startTrainingUC)

	nutritionUC := usecase.NewGetNutritionPlanUseCase(userRepo, planRepo, checkinRepo, cyclePhaseLookup)
	mealSelectionUC := usecase.NewSaveMealSelectionUseCase(planRepo)
	recoveryUC := usecase.NewGetRecoveryUseCase(userRepo, planRepo, cyclePhaseLookup, bannerLib, movesContentLib)

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
	nutritionHandler := httpadapter.NewNutritionHandler(nutritionUC, mealSelectionUC)
	recoveryHandler := httpadapter.NewRecoveryHandler(recoveryUC)
	deviceTokenHandler := httpadapter.NewDeviceTokenHandler(registerDeviceTokenUC)

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

	api.Post("/checkins", checkinHandler.Create)
	api.Get("/checkins/latest", checkinHandler.Latest)
	api.Get("/checkins/status", checkinHandler.Status)

	api.Post("/lifestyle-changes", lifestyleHandler.Report)
	api.Get("/lifestyle-changes", lifestyleHandler.List)

	api.Get("/me", meHandler.GetMe)
	api.Patch("/me", meHandler.UpdateProfile)

	//api.Post("/plans/generate", plansHandler.Generate)
	api.Get("/plans/current", plansHandler.GetCurrent)
	api.Get("/plans/{id}", plansHandler.GetByID)
	//api.Post("/plans/adjust", plansHandler.Adjust)
	api.Get("/plans/week/{week_start}", plansHandler.GetByWeekStart)
	api.Get("/plans/generate/status", plansHandler.GenerateStatus)

	api.Post("/training/resume", trainingHandler.Resume)
	api.Post("/training/generate", trainingHandler.Generate)
	api.Get("/training/generate/status", trainingHandler.GenerateStatus)
	api.Post("/training/validate-arrangement", trainingHandler.ValidateArrangement)
	api.Post("/training/complete-day", trainingHandler.CompleteDay)
	api.Post("/training/save-arrangement", trainingHandler.SaveArrangement)

	api.Get("/nutrition/plan", nutritionHandler.GetPlan)
	api.Post("/nutrition/meal-selection", nutritionHandler.SaveMealSelection)
	api.Get("/recovery/today", recoveryHandler.GetToday)
	api.Post("/plans/phase-feedback", plansHandler.SavePhaseFeedback)

	api.Post("/users/me/device-token", deviceTokenHandler.Upsert)
	// POST /debug/sunday-checkin  — QUITAR EN PRODUCCIÓN
	r.Post("/debug/sunday-checkin", func(w http.ResponseWriter, r *http.Request) {
		if err := sundayCheckinUC.Execute(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

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
