package main

import (
	"context"
	"log"
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
	"viv/internal/adapters/repository"
	"viv/internal/adapters/runner"
	"viv/internal/config"
	"viv/internal/core/recovery"
	"viv/internal/core/rules"
	rulestraining "viv/internal/core/rules/training"
	coretraining "viv/internal/core/training"
	"viv/internal/core/usecase"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
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
	userRepo := repository.NewFirestoreUserRepository(fsClient)
	checkinRepo := repository.NewFirestoreCheckinRepository(fsClient)
	lifestyleRepo := repository.NewFirestoreLifestyleChangeRepository(fsClient)
	planRepo := repository.NewFirestorePlanRepository(fsClient)
	planJobsRepo := repository.NewFirestorePlanJobsRepository(fsClient)

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
	mealGen := openai.NewMealContentGenerator(oaClient)
	generateNutritionUC := usecase.NewGenerateNutritionPlanUsecase(mealGen)

	// ========= Recovery Pipeline =========
	bannerLib, err := recovery.LoadBannerLibrary("internal/content/recovery")
	if err != nil {
		log.Printf("warning: banner library not loaded: %v", err)
		// Don't fatal — banners work with fallback copy
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

	// Plan runner — reuses the same job system as plans
	trainingRunner := runner.NewLocalTrainingPlanRunner(planJobsRepo, generateTrainingUC, generateNutritionUC, userRepo, checkinRepo, cyclePhaseLookup, planRepo, 3*time.Minute)

	// Training usecases — reuse StartPlanGeneration with the training runner
	startTrainingUC := usecase.NewStartPlanGenerationUseCase(planJobsRepo, trainingRunner)
	statusTrainingUC := usecase.NewGetPlanGenerationStatusUseCase(planJobsRepo)
	saveArrangementUC := usecase.NewSaveTrainingArrangementUseCase(planRepo, checkinRepo, trainingLib)

	nutritionUC := usecase.NewGetNutritionPlanUseCase(userRepo, planRepo, checkinRepo, cyclePhaseLookup)
	mealSelectionUC := usecase.NewSaveMealSelectionUseCase(planRepo)
	recoveryUC := usecase.NewGetRecoveryUseCase(userRepo, planRepo, cyclePhaseLookup, bannerLib, movesContentLib)

	phaseFeedbackUC := usecase.NewSavePhaseFeedbackUseCase(planRepo)

	// ========= Handlers =========
	onboardingHandler := httpadapter.NewOnboardingHandler(onboardingUC)
	checkinHandler := httpadapter.NewCheckinHandler(createCheckinUC, latestCheckinUC, statusCheckinUC)
	lifestyleHandler := httpadapter.NewLifestyleHandler(reportLifestyleUC, listLifestyleUC)
	meHandler := httpadapter.NewMeHandler(getMeUC)
	plansHandler := httpadapter.NewPlansHandler(getCurrentPlanUC, getByIDUC, getByWeekStartUC, statusUC, phaseFeedbackUC)
	trainingHandler := httpadapter.NewTrainingHandler(startTrainingUC, statusTrainingUC, trainingEngine, resumeTrainingUC, completeDayUC, saveArrangementUC)
	nutritionHandler := httpadapter.NewNutritionHandler(nutritionUC, mealSelectionUC)
	recoveryHandler := httpadapter.NewRecoveryHandler(recoveryUC)

	// ========= Router config =========
	r := chi.NewRouter()

	// middlewares core
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(90 * time.Second))

	// Public health check
	r.Get("/health", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	/*
	   1) api = router con tus endpoints reales (SIN auth middleware)
	   2) protected = router que aplica FirebaseAuthMiddleware
	   3) /rpc vive en protected y reenvía a api
	*/
	api := chi.NewRouter()

	// Rutas reales (sin auth aquí; el auth lo aplica el router "protected")
	api.Post("/onboarding", onboardingHandler.ServeHTTP)

	api.Post("/checkins", checkinHandler.Create)
	api.Get("/checkins/latest", checkinHandler.Latest)
	api.Get("/checkins/status", checkinHandler.Status)

	api.Post("/lifestyle-changes", lifestyleHandler.Report)
	api.Get("/lifestyle-changes", lifestyleHandler.List)

	api.Get("/me", meHandler.GetMe)

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

	chi.Walk(api, func(method string, route string, handler stdhttp.Handler, middlewares ...func(stdhttp.Handler) stdhttp.Handler) error {
		log.Printf("[api.route] %s %s", method, route)
		return nil
	})

	// Router protegido (aplica auth a TODO lo que montes dentro)
	protected := chi.NewRouter()
	protected.Use(httpadapter.FirebaseAuthMiddleware(authClient))
	protected.Use(httpadapter.EnsureUserMiddleware(userRepo))

	rpcHandler := httpadapter.NewRPCHandler(api)
	protected.Post("/rpc", rpcHandler.Handle)

	protected.Mount("/", api)

	r.Mount("/", protected)

	// ========= 8. Levantar servidor HTTP =========
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

	// ========= 9. Esperar signal y hacer graceful shutdown =========
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

// initFirebaseApp extrae la lógica de inicialización
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
