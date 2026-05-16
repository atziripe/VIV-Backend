package http

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"viv/internal/core/usecase"

	"github.com/getsentry/sentry-go"
)

type createDeviceTokenRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"` // ios/android
	Timezone string `json:"timezone"`
}

type createDeviceTokenResponse struct {
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	Platform  string    `json:"platform"` // ios/android
	Timezone  string    `json:"timezone"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DeviceTokenHandler struct {
	DeviceTokenUC *usecase.RegisterDeviceTokenUseCase
}

func NewDeviceTokenHandler(deviceTokenUC *usecase.RegisterDeviceTokenUseCase) *DeviceTokenHandler {
	return &DeviceTokenHandler{DeviceTokenUC: deviceTokenUC}
}

// POST /users/me/device-token
func (h *DeviceTokenHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createDeviceTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON "+err.Error(), http.StatusBadRequest)
		return
	}

	out, err := h.DeviceTokenUC.Execute(ctx, userID, req.Token, req.Platform, req.Timezone)
	if err != nil {
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.Scope().SetTag("method", r.Method)
			hub.CaptureException(err)
		}
		http.Error(w, "failed to register device token", http.StatusInternalServerError)
		return
	}

	resp := createDeviceTokenResponse{
		UserID:    out.DeviceToken.UserID,
		Token:     out.DeviceToken.Token,
		Platform:  out.DeviceToken.Platform,
		Timezone:  out.DeviceToken.Timezone,
		Active:    out.DeviceToken.Active,
		CreatedAt: out.DeviceToken.CreatedAt,
		UpdatedAt: out.DeviceToken.UpdatedAt,
	}

	log.Printf("[device_token.handler] FCM token: %v", out.DeviceToken.Token)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}
