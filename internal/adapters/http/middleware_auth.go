package http

import (
	"context"
	"log"
	"net/http"
	"strings"

	"firebase.google.com/go/auth"
)

type contextKey string

const userIDContextKey contextKey = "userID"

func FirebaseAuthMiddleware(authClient *auth.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				log.Printf("[auth] missing Authorization header path=%s", r.URL.Path)
				http.Error(w, "missing Authorization header", http.StatusUnauthorized)
				return
			}

			log.Printf("[auth] Authorization header received path=%s", r.URL.Path)

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				log.Printf("[auth] invalid Authorization header format: %q", authHeader)
				http.Error(w, "invalid Authorization header", http.StatusUnauthorized)
				return
			}
			idToken := parts[1]

			// verificar token con Firebase
			token, err := authClient.VerifyIDToken(r.Context(), idToken)
			if err != nil {
				log.Printf("[auth] invalid token: %v", err)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			uid := token.UID
			ctx := context.WithValue(r.Context(), userIDContextKey, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Helper para obtener userID desde el contexto en handlers
func UserIDFromContext(ctx context.Context) (string, bool) {
	uid, ok := ctx.Value(userIDContextKey).(string)
	return uid, ok
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}
