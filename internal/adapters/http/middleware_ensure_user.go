package http

import (
	"log"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// EnsureUserMiddleware also doubles as the one place that observes every
// authenticated request for an existing user — recordActivity stamps
// User.LastActiveAt from here (see RecordUserActivityUseCase) rather than
// needing its own route/polling.
func EnsureUserMiddleware(userRepo usecase.UserRepository, recordActivity *usecase.RecordUserActivityUseCase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, ok := UserIDFromContext(r.Context())
			if !ok || uid == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			u, err := userRepo.GetByID(r.Context(), uid)
			if err != nil {
				log.Printf("[ensure-user] failed to load user=%s: %v", uid, err)
				if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
					hub.Scope().SetTag("endpoint", r.URL.Path)
					hub.Scope().SetTag("method", r.Method)
					hub.CaptureException(err)
				}
				http.Error(w, "failed to load user", http.StatusInternalServerError)
				return
			}

			// Si no existe, lo creamos con defaults mínimos
			if u == nil {
				now := time.Now().UTC()

				newUser := &domain.User{
					ID:                  uid,
					OnboardingCompleted: false,
					CreatedAt:           now,
					UpdatedAt:           now,
					LastActiveAt:        &now,
				}

				// Importante: Create debe ser idempotente (si ya existe, no truena)
				if err := userRepo.Save(r.Context(), newUser); err != nil {
					log.Printf("[ensure-user] failed to create user=%s: %v", uid, err)
					if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
						hub.Scope().SetTag("endpoint", r.URL.Path)
						hub.Scope().SetTag("method", r.Method)
						hub.CaptureException(err)
					}
					http.Error(w, "failed to create user", http.StatusInternalServerError)
					return
				}
			} else if err := recordActivity.Execute(r.Context(), u); err != nil {
				// Non-fatal: activity tracking must never block the actual request.
				log.Printf("[ensure-user] failed to record activity for user=%s: %v", uid, err)
			}

			next.ServeHTTP(w, r)
		})
	}
}
