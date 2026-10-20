package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"viv/internal/core/usecase"
)

type AppleHandler struct {
	UC *usecase.SetAppleRefreshTokenUseCase
}

func NewAppleHandler(uc *usecase.SetAppleRefreshTokenUseCase) *AppleHandler {
	return &AppleHandler{UC: uc}
}

type setAppleCredentialsRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// SetCredentials is POST /me/apple-credentials — see
// usecase.SetAppleRefreshTokenUseCase's doc comment for when the client
// should call this.
func (h *AppleHandler) SetCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req setAppleCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		http.Error(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	if err := h.UC.Execute(ctx, userID, req.RefreshToken); err != nil {
		var notFound usecase.UserNotFoundError
		if errors.As(err, &notFound) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		log.Printf("[apple-credentials] execute error: %v", err)
		http.Error(w, "failed to save apple credentials", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
