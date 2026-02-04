package http

import (
	"net/http"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

func EnsureUserMiddleware(userRepo usecase.UserRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, ok := UserIDFromContext(r.Context())
			if !ok || uid == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			u, err := userRepo.GetByID(r.Context(), uid)
			if err != nil {
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
				}

				// Importante: Create debe ser idempotente (si ya existe, no truena)
				if err := userRepo.Save(r.Context(), newUser); err != nil {
					http.Error(w, "failed to create user", http.StatusInternalServerError)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
