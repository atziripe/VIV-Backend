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
	"viv/internal/core/rules"
	rulestraining "viv/internal/core/rules/training"
	coretraining "viv/internal/core/training"
	"viv/internal/core/usecase"
)

func main() {
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
	planGen := openai.NewGPTPlanGenerator(oaClient, "v1")

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

	// ========= Usecases =========
	onboardingUC := usecase.NewCompleteOnboardingUseCase(userRepo)
	createCheckinUC := usecase.NewCreateCheckinUseCase(checkinRepo, userRepo, "v1")
	latestCheckinUC := usecase.NewGetLatestCheckinUseCase(checkinRepo)
	statusCheckinUC := usecase.NewGetCheckinStatusUseCase(checkinRepo)
	cyclePhaseLookup := usecase.NewCyclePhaseAdapter(userRepo)

	reportLifestyleUC := usecase.NewReportLifestyleChangeUseCase(lifestyleRepo, userRepo)
	listLifestyleUC := usecase.NewListLifestyleChangesUseCase(lifestyleRepo)

	getMeUC := usecase.NewGetCurrentUserUseCase(userRepo)
	generatePlanUC := usecase.NewGeneratePlanUseCase(userRepo, checkinRepo, planRepo, planGen)
	getCurrentPlanUC := usecase.NewGetCurrentPlanUseCase(userRepo, planRepo)

	resumeTrainingUC := usecase.NewResumeTrainingUseCase(userRepo)
	getByIDUC := usecase.NewGetPlanByIDUseCase(userRepo, planRepo)
	adjustUC := usecase.NewAdjustPlanUseCase(userRepo, checkinRepo, planRepo, lifestyleRepo, planGen)
	getByWeekStartUC := usecase.NewGetPlanByWeekStartUseCase(planRepo)

	planRunner := runner.NewLocalPlanGenerationRunner(planJobsRepo, generatePlanUC, 4*time.Minute)
	startUC := usecase.NewStartPlanGenerationUseCase(planJobsRepo, planRunner)
	statusUC := usecase.NewGetPlanGenerationStatusUseCase(planJobsRepo)

	completeDayUC := usecase.NewCompleteTrainingDayUseCase(planRepo)

	// Training runner — reuses the same job system as plans
	trainingRunner := runner.NewLocalTrainingPlanRunner(planJobsRepo, generateTrainingUC, userRepo, checkinRepo, cyclePhaseLookup, planRepo, 2*time.Minute)
	// Training usecases — reuse StartPlanGeneration with the training runner
	startTrainingUC := usecase.NewStartPlanGenerationUseCase(planJobsRepo, trainingRunner)
	statusTrainingUC := usecase.NewGetPlanGenerationStatusUseCase(planJobsRepo)
	saveArrangementUC := usecase.NewSaveTrainingArrangementUseCase(planRepo, checkinRepo, trainingLib)

	// ========= Handlers =========
	onboardingHandler := httpadapter.NewOnboardingHandler(onboardingUC)
	checkinHandler := httpadapter.NewCheckinHandler(createCheckinUC, latestCheckinUC, statusCheckinUC)
	lifestyleHandler := httpadapter.NewLifestyleHandler(reportLifestyleUC, listLifestyleUC)
	meHandler := httpadapter.NewMeHandler(getMeUC)
	plansHandler := httpadapter.NewPlansHandler(generatePlanUC, getCurrentPlanUC, getByIDUC, adjustUC, getByWeekStartUC, startUC, statusUC)
	trainingHandler := httpadapter.NewTrainingHandler(startTrainingUC, statusTrainingUC, trainingEngine, resumeTrainingUC, completeDayUC, saveArrangementUC)

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
	chi.Walk(api, func(method string, route string, handler stdhttp.Handler, middlewares ...func(stdhttp.Handler) stdhttp.Handler) error {
		log.Printf("[api.route] %s %s", method, route)
		return nil
	})

	api.Post("/plans/generate", plansHandler.Generate)
	api.Get("/plans/current", plansHandler.GetCurrent)
	api.Get("/plans/{id}", plansHandler.GetByID)
	api.Post("/plans/adjust", plansHandler.Adjust)
	api.Get("/plans/week/{week_start}", plansHandler.GetByWeekStart)
	api.Get("/plans/generate/status", plansHandler.GenerateStatus)

	api.Post("/training/resume", trainingHandler.Resume)
	api.Post("/training/generate", trainingHandler.Generate)
	api.Get("/training/generate/status", trainingHandler.GenerateStatus)
	api.Post("/training/validate-arrangement", trainingHandler.ValidateArrangement)
	api.Post("/training/complete-day", trainingHandler.CompleteDay)
	api.Post("/training/save-arrangement", trainingHandler.SaveArrangement)

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
		Handler:      r,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("VIV backend listening on :%s", cfg.HTTPPort)
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
