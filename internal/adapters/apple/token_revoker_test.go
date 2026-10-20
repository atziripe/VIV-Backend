package apple

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
)

func generateTestKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating test key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshaling test key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

func TestNewTokenRevoker_ValidKey(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	revoker, err := NewTokenRevoker("TEAM123", "KEY456", "com.example.app", keyPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revoker == nil {
		t.Fatal("expected non-nil revoker")
	}
}

func TestNewTokenRevoker_MalformedPEM(t *testing.T) {
	_, err := NewTokenRevoker("TEAM123", "KEY456", "com.example.app", "not a pem block")
	if err == nil {
		t.Fatal("expected error for malformed PEM")
	}
}

func TestNewTokenRevoker_UnparseablePEMBody(t *testing.T) {
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: []byte("not real key bytes")}
	keyPEM := string(pem.EncodeToMemory(block))

	_, err := NewTokenRevoker("TEAM123", "KEY456", "com.example.app", keyPEM)
	if err == nil {
		t.Fatal("expected error for a PEM block that isn't a parseable PKCS8 key")
	}
}

func TestNewTokenRevoker_NonECKey(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA test key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatalf("marshaling RSA test key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	keyPEM := string(pem.EncodeToMemory(block))

	_, err = NewTokenRevoker("TEAM123", "KEY456", "com.example.app", keyPEM)
	if err == nil {
		t.Fatal("expected error for a valid but non-EC (RSA) key")
	}
}

func TestClientSecret_ProducesValidJWS(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	revoker, err := NewTokenRevoker("TEAM123", "KEY456", "com.example.app", keyPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	token, err := revoker.clientSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected a 3-part JWS, got %d parts", len(parts))
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decoding header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshaling header: %v", err)
	}
	if header["alg"] != "ES256" || header["kid"] != "KEY456" {
		t.Fatalf("unexpected header: %+v", header)
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decoding claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatalf("unmarshaling claims: %v", err)
	}
	if claims["iss"] != "TEAM123" || claims["sub"] != "com.example.app" || claims["aud"] != "https://appleid.apple.com" {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decoding signature: %v", err)
	}
	if len(sig) != 64 {
		t.Fatalf("expected a 64-byte R||S signature, got %d bytes", len(sig))
	}
}
