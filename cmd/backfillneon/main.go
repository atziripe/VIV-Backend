// Command backfillneon copies every historical Firestore record into Neon,
// once, before any read path is cut over from Firestore to Neon (see
// repository.RunNeonBackfill's doc comment for why this is necessary — the
// dual-write path only ever mirrors data written after it went live).
//
// Usage:
//
//	DATABASE_URL="postgresql://..." GOOGLE_APPLICATION_CREDENTIALS=/path/to/creds.json \
//	    go run ./cmd/backfillneon -project your-firebase-project-id
//
// Always run with the default -dry-run=true first and check the printed
// counts look right before re-running with -dry-run=false.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	fb "firebase.google.com/go"
	"google.golang.org/api/option"

	"viv/internal/adapters/repository"
)

func main() {
	projectID := flag.String("project", os.Getenv("FIREBASE_PROJECT_ID"), "Firebase project ID (or set FIREBASE_PROJECT_ID)")
	dryRun := flag.Bool("dry-run", true, "read and count everything without writing to Neon")
	flag.Parse()

	if *projectID == "" {
		log.Fatal("backfillneon: -project (or FIREBASE_PROJECT_ID) is required")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("backfillneon: DATABASE_URL is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := []option.ClientOption{}
	if credsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON"); credsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(credsJSON)))
	} else if credsFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); credsFile != "" {
		opts = append(opts, option.WithCredentialsFile(credsFile))
	}

	app, err := fb.NewApp(ctx, &fb.Config{ProjectID: *projectID}, opts...)
	if err != nil {
		log.Fatalf("backfillneon: init firebase app: %v", err)
	}

	fsClient, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalf("backfillneon: init firestore client: %v", err)
	}
	defer fsClient.Close()

	pool, err := repository.NewNeonPool(ctx, dbURL)
	if err != nil {
		log.Fatalf("backfillneon: connect to neon: %v", err)
	}
	defer pool.Close()

	if *dryRun {
		log.Println("backfillneon: DRY RUN — nothing will be written to Neon")
	} else {
		log.Println("backfillneon: LIVE RUN — writing to Neon")
	}

	report, err := repository.RunNeonBackfill(ctx, fsClient, pool, *dryRun)
	if err != nil {
		log.Fatalf("backfillneon: %v", err)
	}

	log.Printf(`backfillneon: done
  users              %d
  checkins (legacy)  %d
  plans (legacy)     %d
  device_tokens      %d
  lifestyle_changes  %d
  weekly_plan_drafts %d
  exercise_pins      %d
  daily_checkins     %d
  session_logs       %d
  nutrition_plans    %d
  recovery_actions   %d
  errors             %d`,
		report.Users, report.Checkins, report.Plans, report.DeviceTokens,
		report.LifestyleChanges, report.WeeklyPlanDrafts, report.ExercisePins,
		report.DailyCheckins, report.SessionLogs, report.NutritionPlans,
		report.RecoveryActions, len(report.Errors))

	for _, e := range report.Errors {
		log.Printf("backfillneon: ERROR: %s", e)
	}
	if len(report.Errors) > 0 {
		os.Exit(1)
	}
}
