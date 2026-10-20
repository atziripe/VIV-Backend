package apple

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"viv/internal/core/usecase"
)

var _ usecase.AppleTokenRevoker = (*Client)(nil)
var _ usecase.AppleCodeExchanger = (*Client)(nil)

const (
	revokeEndpoint = "https://appleid.apple.com/auth/revoke"
	tokenEndpoint  = "https://appleid.apple.com/auth/token"
)

// clientSecretTTL is how long the client_secret JWT minted per call is
// valid for — generated fresh on every call rather than cached, so this
// just needs to comfortably outlast one HTTP round trip.
const clientSecretTTL = 5 * time.Minute

// Client talks to Apple's own servers for the two "Sign in with Apple"
// operations VIV needs: exchanging a client-captured authorization_code
// for a refresh_token (ExchangeAuthorizationCode), and revoking that
// refresh_token on account deletion (RevokeRefreshToken) — see
// usecase.AppleCodeExchanger/usecase.AppleTokenRevoker's doc comments for
// why each is optional/best-effort from its caller's point of view. Both
// operations authenticate with the same signed client_secret JWT, built
// fresh per call from VIV's "Sign in with Apple" Developer credentials:
// Team ID, Key ID, the EC private key (the format Apple's
// "AuthKey_XXXXXXXXXX.p8" files use), and the Services/App ID used as the
// client id — see NewClient.
type Client struct {
	teamID     string
	keyID      string
	clientID   string
	privateKey *ecdsa.PrivateKey
	httpClient *http.Client
}

// NewClient parses privateKeyPEM once at startup and returns an error if
// it's malformed — fail fast at wiring time rather than on the first real
// call to Apple.
func NewClient(teamID, keyID, clientID, privateKeyPEM string) (*Client, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("apple client: no PEM block found in private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apple client: parsing private key: %w", err)
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("apple client: private key is not an EC key")
	}

	return &Client{
		teamID: teamID, keyID: keyID, clientID: clientID,
		privateKey: ecKey, httpClient: http.DefaultClient,
	}, nil
}

// tokenExchangeResponse is Apple's /auth/token response — only
// RefreshToken is used; the rest exists so decoding doesn't choke on the
// other fields Apple sends.
type tokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// ExchangeAuthorizationCode trades a client-captured authorization_code
// (single-use, ~5 minutes to live) for a refresh_token. Apple issues a
// fresh authorization_code on every Sign in with Apple, not just the
// first, but doesn't always return a new refresh_token on every exchange
// — an empty return value here means "nothing new," not an error; the
// caller is responsible for keeping whatever refresh_token it already had
// in that case (see ExchangeAppleAuthorizationCodeUseCase).
func (c *Client) ExchangeAuthorizationCode(ctx context.Context, authorizationCode string) (string, error) {
	clientSecret, err := c.clientSecret()
	if err != nil {
		return "", fmt.Errorf("apple client: building client secret: %w", err)
	}

	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {authorizationCode},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("apple client: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("apple client: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("apple client: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("apple client: decoding token response: %w", err)
	}

	return tokenResp.RefreshToken, nil
}

func (c *Client) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	clientSecret, err := c.clientSecret()
	if err != nil {
		return fmt.Errorf("apple client: building client secret: %w", err)
	}

	form := url.Values{
		"client_id":       {c.clientID},
		"client_secret":   {clientSecret},
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeEndpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return fmt.Errorf("apple client: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("apple client: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("apple client: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// clientSecret builds the ES256-signed JWT Apple requires in place of a
// static client secret — see Apple's "Generate and validate tokens" guide.
func (c *Client) clientSecret() (string, error) {
	now := time.Now().UTC()

	header := map[string]string{"alg": "ES256", "kid": c.keyID}
	claims := map[string]any{
		"iss": c.teamID,
		"iat": now.Unix(),
		"exp": now.Add(clientSecretTTL).Unix(),
		"aud": "https://appleid.apple.com",
		"sub": c.clientID,
	}

	headerB64, err := encodeSegment(header)
	if err != nil {
		return "", err
	}
	claimsB64, err := encodeSegment(claims)
	if err != nil {
		return "", err
	}

	signingInput := headerB64 + "." + claimsB64
	hash := sha256.Sum256([]byte(signingInput))

	rVal, sVal, err := ecdsa.Sign(rand.Reader, c.privateKey, hash[:])
	if err != nil {
		return "", fmt.Errorf("signing: %w", err)
	}

	// JWS expects a fixed-size R||S concatenation (32 bytes each for
	// P-256), not ecdsa's variable-length big.Int output.
	sigBytes := make([]byte, 64)
	rVal.FillBytes(sigBytes[:32])
	sVal.FillBytes(sigBytes[32:])
	sigB64 := base64.RawURLEncoding.EncodeToString(sigBytes)

	return signingInput + "." + sigB64, nil
}

func encodeSegment(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
