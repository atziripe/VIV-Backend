package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"viv/internal/core/usecase"
)

var _ usecase.ReEngagementEmailSender = (*BrevoReEngagementEmailSender)(nil)

const brevoSendEndpoint = "https://api.brevo.com/v3/smtp/email"

// BrevoReEngagementEmailSender sends VIV's inactivity re-engagement email
// through Brevo's transactional email API, using a template managed in
// Brevo's own dashboard — this adapter only ever supplies the recipient
// and template params, never the email copy itself.
type BrevoReEngagementEmailSender struct {
	apiKey     string
	templateID int
	httpClient *http.Client
}

func NewBrevoReEngagementEmailSender(apiKey string, templateID int) *BrevoReEngagementEmailSender {
	return &BrevoReEngagementEmailSender{apiKey: apiKey, templateID: templateID, httpClient: http.DefaultClient}
}

type brevoRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoSendRequest struct {
	To         []brevoRecipient  `json:"to"`
	TemplateID int               `json:"templateId"`
	Params     map[string]string `json:"params,omitempty"`
}

func (s *BrevoReEngagementEmailSender) SendReEngagementEmail(ctx context.Context, email, name string) error {
	body := brevoSendRequest{
		To:         []brevoRecipient{{Email: email, Name: name}},
		TemplateID: s.templateID,
		Params:     map[string]string{"name": name},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("brevo: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, brevoSendEndpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("brevo: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("api-key", s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("brevo: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("brevo: unexpected status %d", resp.StatusCode)
	}
	return nil
}
