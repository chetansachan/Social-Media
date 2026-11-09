package dispatchers

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
)

// LogPushProvider is the safe local-development default. It never contacts an
// external provider, but exercises the full worker/token-selection pipeline.
type LogPushProvider struct {
	logger *log.Logger
}

func NewLogPushProvider(logger *log.Logger) *LogPushProvider {
	if logger == nil {
		logger = log.Default()
	}
	return &LogPushProvider{logger: logger}
}

func (p *LogPushProvider) Send(ctx context.Context, device models.DeviceToken, message models.PushMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.logger.Printf("push[log] -> user=%s platform=%s token=%s title=%q body=%q",
		message.RecipientID, device.Platform, maskToken(device.Token), truncate(message.Title, 120), truncate(message.Body, 160))
	return nil
}

// -----------------------------------------------------------------------------
// Firebase Cloud Messaging (HTTP v1)
// -----------------------------------------------------------------------------

type fcmServiceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
	ProjectID   string `json:"project_id"`
}

// FCMProvider implements the FCM HTTP v1 send API. It signs a JWT assertion
// with the service-account key, exchanges it for an OAuth2 access token, and
// caches that token until shortly before expiry.
type FCMProvider struct {
	client         *http.Client
	projectID      string
	tokenURI       string
	clientEmail    string
	privateKey     *rsa.PrivateKey
	logger         *log.Logger
	mu             sync.Mutex
	accessToken    string
	tokenExpiresAt time.Time
}

func NewFCMProvider(credentialsFile, projectID string, client *http.Client, logger *log.Logger) (*FCMProvider, error) {
	if strings.TrimSpace(credentialsFile) == "" {
		return nil, fmt.Errorf("FCM credentials file is required")
	}
	raw, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read FCM credentials: %w", err)
	}

	var account fcmServiceAccount
	if err := json.Unmarshal(raw, &account); err != nil {
		return nil, fmt.Errorf("failed to parse FCM credentials: %w", err)
	}
	if account.ClientEmail == "" || account.PrivateKey == "" {
		return nil, fmt.Errorf("FCM credentials must include client_email and private_key")
	}
	if projectID == "" {
		projectID = account.ProjectID
	}
	if projectID == "" {
		return nil, fmt.Errorf("FCM project id is required")
	}
	if account.TokenURI == "" {
		account.TokenURI = "https://oauth2.googleapis.com/token"
	}

	key, err := parseRSAPrivateKey(account.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse FCM service-account key: %w", err)
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = log.Default()
	}

	return &FCMProvider{
		client:      client,
		projectID:   projectID,
		tokenURI:    account.TokenURI,
		clientEmail: account.ClientEmail,
		privateKey:  key,
		logger:      logger,
	}, nil
}

func (p *FCMProvider) Send(ctx context.Context, device models.DeviceToken, message models.PushMessage) error {
	accessToken, err := p.token(ctx)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"message": map[string]any{
			"token": device.Token,
			"notification": map[string]string{
				"title": truncate(message.Title, 120),
				"body":  truncate(message.Body, 200),
			},
			"data": map[string]string{
				"notification_id": message.NotificationID,
				"action_url":      message.ActionURL,
			},
			"android": map[string]any{
				"priority": "high",
				"notification": map[string]string{
					"sound": "default",
				},
			},
			"apns": map[string]any{
				"payload": map[string]any{
					"aps": map[string]any{
						"sound": "default",
						"badge": message.UnreadCount,
					},
				},
			},
		},
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode FCM payload: %w", err)
	}

	endpoint := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", p.projectID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("failed to build FCM request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("FCM request failed: %w", err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone:
		return models.ErrTokenUnregistered
	case response.StatusCode == http.StatusBadRequest && fcmTokenError(body):
		return models.ErrTokenUnregistered
	default:
		return providerError("FCM", response.StatusCode, body)
	}
}

func fcmTokenError(body []byte) bool {
	text := strings.ToUpper(string(body))
	return strings.Contains(text, "UNREGISTERED") ||
		strings.Contains(text, "REGISTRATION TOKEN") ||
		strings.Contains(text, "INVALID_ARGUMENT")
}

func (p *FCMProvider) token(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.accessToken != "" && time.Now().Before(p.tokenExpiresAt.Add(-time.Minute)) {
		return p.accessToken, nil
	}

	now := time.Now()
	assertion, err := signRS256(p.privateKey, map[string]any{
		"iss":   p.clientEmail,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud":   p.tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to sign FCM assertion: %w", err)
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to build FCM token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := p.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("FCM token exchange failed: %w", err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", providerError("FCM token endpoint", response.StatusCode, body)
	}

	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", fmt.Errorf("failed to decode FCM token response: %w", err)
	}
	if tokenResponse.AccessToken == "" {
		return "", fmt.Errorf("FCM token response did not include an access token")
	}

	if tokenResponse.ExpiresIn <= 0 {
		tokenResponse.ExpiresIn = 3600
	}
	p.accessToken = tokenResponse.AccessToken
	p.tokenExpiresAt = now.Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
	return p.accessToken, nil
}

// -----------------------------------------------------------------------------
// Apple Push Notification service (HTTP/2 provider API)
// -----------------------------------------------------------------------------

// APNsProvider implements the token-based APNs HTTP/2 API. net/http negotiates
// HTTP/2 automatically for https endpoints, so no separate transport is needed.
type APNsProvider struct {
	client     *http.Client
	keyID      string
	teamID     string
	topic      string
	endpoint   string
	privateKey *ecdsa.PrivateKey
	logger     *log.Logger
	mu         sync.Mutex
	jwt        string
	jwtExpiry  time.Time
}

func NewAPNsProvider(keyFile, keyID, teamID, topic string, sandbox bool, client *http.Client) (*APNsProvider, error) {
	if keyFile == "" || keyID == "" || teamID == "" || topic == "" {
		return nil, fmt.Errorf("APNs key file, key id, team id, and topic are required")
	}

	raw, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read APNs key: %w", err)
	}
	privateKey, err := parseECPrivateKey(string(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to parse APNs key: %w", err)
	}

	endpoint := "https://api.push.apple.com"
	if sandbox {
		endpoint = "https://api.sandbox.push.apple.com"
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &APNsProvider{
		client:     client,
		keyID:      keyID,
		teamID:     teamID,
		topic:      topic,
		endpoint:   endpoint,
		privateKey: privateKey,
		logger:     log.Default(),
	}, nil
}

func (p *APNsProvider) Send(ctx context.Context, device models.DeviceToken, message models.PushMessage) error {
	authToken, err := p.authorizationToken()
	if err != nil {
		return err
	}

	payload := map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{
				"title": truncate(message.Title, 120),
				"body":  truncate(message.Body, 200),
			},
			"badge": message.UnreadCount,
			"sound": "default",
		},
		"notification_id": message.NotificationID,
		"action_url":      message.ActionURL,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode APNs payload: %w", err)
	}

	endpoint := p.endpoint + "/3/device/" + device.Token
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("failed to build APNs request: %w", err)
	}
	request.Header.Set("authorization", "bearer "+authToken)
	request.Header.Set("apns-topic", p.topic)
	request.Header.Set("apns-push-type", "alert")
	request.Header.Set("apns-priority", "10")
	request.Header.Set("content-type", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("APNs request failed: %w", err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
	switch {
	case response.StatusCode == http.StatusOK:
		return nil
	case response.StatusCode == http.StatusGone:
		return models.ErrTokenUnregistered
	case response.StatusCode == http.StatusBadRequest && apnsTokenError(body):
		return models.ErrTokenUnregistered
	default:
		return providerError("APNs", response.StatusCode, body)
	}
}

func apnsTokenError(body []byte) bool {
	var response struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return false
	}
	switch response.Reason {
	case "BadDeviceToken", "Unregistered", "DeviceTokenNotForTopic":
		return true
	default:
		return false
	}
}

func (p *APNsProvider) authorizationToken() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if p.jwt != "" && now.Before(p.jwtExpiry) {
		return p.jwt, nil
	}

	token, err := signES256(p.privateKey, p.keyID, map[string]any{
		"iss": p.teamID,
		"iat": now.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to sign APNs token: %w", err)
	}

	p.jwt = token
	p.jwtExpiry = now.Add(50 * time.Minute)
	return token, nil
}

// -----------------------------------------------------------------------------
// JWT / key helpers
// -----------------------------------------------------------------------------

func signRS256(privateKey *rsa.PrivateKey, claims map[string]any) (string, error) {
	header, err := encodeJWTPart(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := encodeJWTPart(claims)
	if err != nil {
		return "", err
	}

	signingInput := header + "." + payload
	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func signES256(privateKey *ecdsa.PrivateKey, keyID string, claims map[string]any) (string, error) {
	header, err := encodeJWTPart(map[string]string{"alg": "ES256", "kid": keyID, "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := encodeJWTPart(claims)
	if err != nil {
		return "", err
	}

	signingInput := header + "." + payload
	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		return "", err
	}

	byteLen := (privateKey.Curve.Params().BitSize + 7) / 8
	signature := make([]byte, byteLen*2)
	r.FillBytes(signature[:byteLen])
	s.FillBytes(signature[byteLen:])

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func encodeJWTPart(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func parseRSAPrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.ReplaceAll(pemData, `\n`, "\n")))
	if block == nil {
		return nil, fmt.Errorf("PEM block not found")
	}

	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if key, ok := parsed.(*rsa.PrivateKey); ok {
			return key, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("unsupported RSA private key format")
}

func parseECPrivateKey(pemData string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.ReplaceAll(pemData, `\n`, "\n")))
	if block == nil {
		return nil, fmt.Errorf("PEM block not found")
	}

	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if key, ok := parsed.(*ecdsa.PrivateKey); ok {
			return key, nil
		}
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("unsupported EC private key format")
}

func maskToken(token string) string {
	if len(token) <= 12 {
		return "***"
	}
	return token[:6] + "..." + token[len(token)-4:]
}
