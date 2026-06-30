package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// mockTokenSource is a mock implementation of oauth2.TokenSource.
// It allows returning predefined token objects or errors for predictable testing.
type mockTokenSource struct {
	token *oauth2.Token
	err   error
}

// Token implements the oauth2.TokenSource interface.
func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	return m.token, m.err
}

// TestGenerateTokenWithOptions verifies that GenerateTokenWithOptions correctly fetches,
// processes, and encodes the token components into the mandated JWT Kafka SASL structure.
func TestGenerateTokenWithOptions(t *testing.T) {
	// 1. Setup a mock HTTP server to simulate Google's UserInfo API endpoint.
	// It will verify that the token provider sends the Bearer token in the request header
	// and returns a mock GoogleUserInfo JSON payload.
	expectedEmail := "test-service-account@test-project.iam.gserviceaccount.com"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that the HTTP Authorization header starts with 'Bearer '
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Respond with mock user info containing the target service account email
		userInfo := GoogleUserInfo{
			Sub:           "1234567890",
			Email:         expectedEmail,
			EmailVerified: true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(userInfo)
	}))
	defer server.Close()

	// 2. Setup a mock TokenSource that returns a stable OAuth2 access token with a fixed expiration time.
	expiry := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	accessToken := "mock-access-token-12345"
	mts := &mockTokenSource{
		token: mock2_token(accessToken, expiry),
	}

	// 3. Invoke GenerateTokenWithOptions with our mock TokenSource, HTTP Client,
	// and mock UserInfo endpoint URL to avoid hitting the actual Google APIs.
	ctx := context.Background()
	tokenStr, expiryMs, err := GenerateTokenWithOptions(
		ctx,
		WithTokenSource(mts),
		WithHTTPClient(server.Client()),
		WithUserInfoURL(server.URL),
	)

	if err != nil {
		t.Fatalf("GenerateTokenWithOptions failed: %v", err)
	}

	// Verify that the returned expiration timestamp in milliseconds is calculated correctly.
	expectedExpiryMs := expiry.UnixNano() / int64(time.Millisecond)
	if expiryMs != expectedExpiryMs {
		t.Errorf("expected expiryMs %d, got %d", expectedExpiryMs, expiryMs)
	}

	// 4. Verify that the final constructed token structure matches the specifications.
	// The token must consist of 3 Base64 encoded parts connected by dots.
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in token, got %d", len(parts))
	}

	// Step 4a: Decode and verify JWT Header
	decodedHeader, err := decodeB64(parts[0])
	if err != nil {
		t.Fatalf("failed to decode header: %v", err)
	}
	var header map[string]any
	if err := json.Unmarshal(decodedHeader, &header); err != nil {
		t.Fatalf("failed to unmarshal header: %v", err)
	}
	if header["typ"] != "JWT" || header["alg"] != "GOOG_OAUTH2_TOKEN" {
		t.Errorf("unexpected header values: %v", header)
	}

	// Step 4b: Decode and verify JWT Payload
	decodedPayload, err := decodeB64(parts[1])
	if err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(decodedPayload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload["iss"] != "Google" || payload["sub"] != expectedEmail || payload["scope"] != "kafka" {
		t.Errorf("unexpected payload values: %v", payload)
	}
	if int64(payload["exp"].(float64)) != expiry.Unix() {
		t.Errorf("expected exp %d, got %v", expiry.Unix(), payload["exp"])
	}

	// Step 4c: Decode and verify raw Google Access Token Part
	decodedAccessToken, err := decodeB64(parts[2])
	if err != nil {
		t.Fatalf("failed to decode access token part: %v", err)
	}
	if string(decodedAccessToken) != accessToken {
		t.Errorf("expected access token to be %s, got %s", accessToken, string(decodedAccessToken))
	}
}

// mock2_token is a helper function to instantiate an oauth2.Token.
func mock2_token(accessToken string, expiry time.Time) *oauth2.Token {
	return &oauth2.Token{
		AccessToken: accessToken,
		Expiry:      expiry,
	}
}

// decodeB64 is a testing helper to decode padding-trimmed Base64 strings.
// It dynamically appends '=' characters as necessary to restore standard Base64 padding before decoding.
func decodeB64(s string) ([]byte, error) {
	if len(s)%4 != 0 {
		s += strings.Repeat("=", 4-(len(s)%4))
	}
	return base64.StdEncoding.DecodeString(s)
}
