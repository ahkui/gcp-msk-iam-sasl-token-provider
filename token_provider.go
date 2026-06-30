// Package provider provides a GCP SASL OAUTHBEARER Token Provider for Google Cloud Managed Service for Apache Kafka.
package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// headerPayload is the predefined JSON Web Token (JWT) header required by GCP Managed Service for Apache Kafka.
// It specifies 'typ' as 'JWT' and 'alg' as 'GOOG_OAUTH2_TOKEN', which indicates that the third part
// of this JWT structure is a raw Google OAuth2 Access Token.
var headerPayload, _ = json.Marshal(map[string]any{"typ": "JWT", "alg": "GOOG_OAUTH2_TOKEN"})

// Option defines a functional option for configuring the token generation process.
type Option func(*options)

// options holds the configuration for retrieving and formatting the Google MSK SASL token.
type options struct {
	// client is the HTTP client used to query the Google UserInfo API.
	client *http.Client
	// tokenSource is the OAuth2 source used to fetch the Google Access Token.
	tokenSource oauth2.TokenSource
	// userInfoURL is the Google UserInfo API endpoint.
	userInfoURL string
}

// WithHTTPClient configures a custom HTTP client for requesting user info.
// This is useful if requests need to go through a proxy or require custom timeouts.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		o.client = client
	}
}

// WithTokenSource configures a custom token source for retrieving OAuth2 tokens.
// This allows developers to pass custom credential sources (e.g., from a specific service account file).
func WithTokenSource(ts oauth2.TokenSource) Option {
	return func(o *options) {
		o.tokenSource = ts
	}
}

// WithUserInfoURL configures a custom user info endpoint URL.
// Typically used to point to a local mock server during unit testing or integration testing.
func WithUserInfoURL(url string) Option {
	return func(o *options) {
		o.userInfoURL = url
	}
}

// GoogleUserInfo represents the JSON response structure returned by the Google OAuth2 UserInfo API.
type GoogleUserInfo struct {
	// Sub is the unique identifier for the Google user or service account.
	Sub string `json:"sub"`
	// Email is the service account or user email address associated with the credentials.
	Email string `json:"email"`
	// EmailVerified indicates whether the email address has been verified by Google.
	EmailVerified bool `json:"email_verified"`
}

// GenerateToken generates the base64-encoded Kafka OAUTHBEARER token for GCP Managed Service for Apache Kafka using Google Application Default Credentials.
// It retrieves the GCP OAuth2 Access Token, queries the account identity (email) and formats them into a JWT structure required by GCP Managed Service for Apache Kafka.
// It returns the base64-encoded token string, the expiration time in milliseconds (since epoch), and any error occurred.
func GenerateToken(ctx context.Context) (string, int64, error) {
	return GenerateTokenWithOptions(ctx)
}

// GenerateTokenWithOptions generates the base64-encoded token with custom options.
// This allows customizing the OAuth2 TokenSource, HTTP Client, or UserInfo URL for testing or enterprise proxy purposes.
func GenerateTokenWithOptions(ctx context.Context, opts ...Option) (string, int64, error) {
	// 1. Initialize default options
	o := &options{
		client:      http.DefaultClient,
		userInfoURL: "https://www.googleapis.com/oauth2/v3/userinfo",
	}
	for _, opt := range opts {
		opt(o)
	}

	// 2. Resolve Google Token Source if not explicitly provided
	const scope = "https://www.googleapis.com/auth/cloud-platform"
	if o.tokenSource == nil {
		var err error
		o.tokenSource, err = google.DefaultTokenSource(ctx, scope)
		if err != nil {
			return "", 0, fmt.Errorf("failed to create default token source: %w", err)
		}
	}

	// 3. Fetch Google OAuth2 Access Token
	token, err := o.tokenSource.Token()
	if err != nil {
		return "", 0, fmt.Errorf("failed to retrieve token: %w", err)
	}

	if !token.Valid() {
		return "", 0, fmt.Errorf("received invalid token")
	}

	// 4. Query the email identity of the current token using Google UserInfo API
	email, err := getEmail(ctx, o.client, o.userInfoURL, token.AccessToken)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get user info: %w", err)
	}

	// 5. Construct token expiry time in milliseconds for the Kafka client
	expiryTime := token.Expiry
	expiryMs := expiryTime.UnixNano() / int64(time.Millisecond)

	// 6. Build the JWT payload required by GCP MSK SASL OAUTHBEARER authentication
	payload := map[string]any{
		"exp":   expiryTime.Unix(),
		"iat":   time.Now().UTC().Unix(),
		"iss":   "Google",
		"sub":   email,
		"scope": "kafka",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 7. Assemble the final token. The structure is 3 parts connected by dots:
	// Part 1: Base64UrlEncoded JWT Header
	// Part 2: Base64UrlEncoded JWT Payload
	// Part 3: Base64UrlEncoded Google Access Token
	constructedToken := strings.Join([]string{
		b64enc(headerPayload),
		b64enc(payloadJSON),
		b64enc([]byte(token.AccessToken)),
	}, ".")

	return constructedToken, expiryMs, nil
}

// getEmail queries Google's UserInfo endpoint to resolve the email address of the current credentials.
func getEmail(ctx context.Context, client *http.Client, url string, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", "Bearer "+token)

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google api returned bad status: %s", res.Status)
	}

	var userInfo GoogleUserInfo
	if err := json.NewDecoder(res.Body).Decode(&userInfo); err != nil {
		return "", fmt.Errorf("failed to decode user info: %w", err)
	}

	return userInfo.Email, nil
}

// b64enc encodes the given byte slice using Base64 Standard Encoding with all trailing padding characters ('=') removed.
// GCP MSK authentication mandates that padding '=' must be trimmed from the Base64 parts.
func b64enc(s []byte) string {
	return strings.TrimRight((base64.StdEncoding.EncodeToString(s)), "=")
}
