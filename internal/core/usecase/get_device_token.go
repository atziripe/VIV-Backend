package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"viv/internal/core/domain"
)

type RegisterDeviceTokenUseCase struct {
	DeviceToken DeviceTokenRepository
}

func NewRegisterDeviceTokenUseCase(repo DeviceTokenRepository) *RegisterDeviceTokenUseCase {
	return &RegisterDeviceTokenUseCase{
		DeviceToken: repo,
	}
}

type RegisterDeviceTokenInput struct {
	UserID   string
	Token    string
	Platform string // ios/android
	Timezone string
}

type RegisterDeviceTokenOutput struct {
	DeviceToken *domain.DeviceToken
}

func (uc *RegisterDeviceTokenUseCase) Execute(ctx context.Context, userID, token, platform string, timezone string) (*RegisterDeviceTokenOutput, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user_id_required")
	}
	if token == "" {
		return nil, errors.New("device_token_required")
	}
	if platform != "ios" && platform != "android" {
		return nil, errors.New("invalid_platform")
	}

	now := time.Now().UTC()

	device_token := &domain.DeviceToken{
		UserID:    userID,
		Token:     token,
		Platform:  platform,
		Timezone:  timezone,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := uc.DeviceToken.Upsert(ctx, device_token); err != nil {
		return nil, err
	}

	return &RegisterDeviceTokenOutput{DeviceToken: device_token}, nil
}
