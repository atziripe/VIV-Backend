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
	"net/http"
	"net/url"
	"time"

	"viv/internal/core/usecase"
)

var _ usecase.AppleTokenRevoker = (*TokenRevoker)(nil)

const revokeEndpoint = "https://appleid.apple.com/auth/revoke"

// clientSecretTTL is how long the client_secret JWT minted per call is
// valid for — generated fresh on every revoke rather than cached, so this
// just needs to comfortably outlast one HTTP round trip.
const clientSecretTTL = 5 * time.Minute

// TokenRevoker revokes a Sign in with Apple refresh token — see
// usecase.AppleTokenRevoker's doc comment for why this exists and why
// it's optional/best-effort. Needs VIV's "Sign in with Apple" Developer
// credentials: Team ID, Key ID, the EC private key (the format Apple's
// "AuthKey_XXXXXXXXXX.p8" files use), and the Services/App ID used as the
// client id — see NewTokenRevoker.
type TokenRevoker struct {
	teamID     string
	keyID      string
	clientID   string
	privateKey *ecdsa.PrivateKey
	httpClient *http.Client
}

// NewTokenRevoker parses privateKeyPEM once at startup and returns an
// error if it's malformed — fail fast at wiring time rather than on the
// first real revocation attempt.
func NewTokenRevoker(teamID, keyID, clientID, privateKeyPEM string) (*TokenRevoker, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("apple token revoker: no PEM block found in private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apple token revoker: parsing private key: %w", err)
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("apple token revoker: private key is not an EC key")
	}

	return &TokenRevoker{
		teamID: teamID, keyID: keyID, clientID: clientID,
		privateKey: ecKey, httpClient: http.DefaultClient,
	}, nil
}

func (r *TokenRevoker) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	clientSecret, err := r.clientSecret()
	if err != nil {
		return fmt.Errorf("apple token revoker: building client secret: %w", err)
	}

	form := url.Values{
		"client_id":       {r.clientID},
		"client_secret":   {clientSecret},
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeEndpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return fmt.Errorf("apple token revoker: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("apple token revoker: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("apple token revoker: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// clientSecret builds the ES256-signed JWT Apple requires in place of a
// static client secret — see Apple's "Generate and validate tokens" guide.
func (r *TokenRevoker) clientSecret() (string, error) {
	now := time.Now().UTC()

	header := map[string]string{"alg": "ES256", "kid": r.keyID}
	claims := map[string]any{
		"iss": r.teamID,
		"iat": now.Unix(),
		"exp": now.Add(clientSecretTTL).Unix(),
		"aud": "https://appleid.apple.com",
		"sub": r.clientID,
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

	rVal, sVal, err := ecdsa.Sign(rand.Reader, r.privateKey, hash[:])
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
