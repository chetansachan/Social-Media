package dispatchers

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
	"github.com/google/uuid"
)

type fakeDeviceLister struct {
	mu        sync.Mutex
	tokens    []models.DeviceToken
	marked    []string
	listError error
}

func (f *fakeDeviceLister) ListActive(ctx context.Context, userID uuid.UUID) ([]models.DeviceToken, error) {
	if f.listError != nil {
		return nil, f.listError
	}
	return f.tokens, nil
}

func (f *fakeDeviceLister) MarkInactive(ctx context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marked = append(f.marked, token)
	return nil
}

type fakePushProvider struct {
	mu    sync.Mutex
	sends int
	err   error
}

func (f *fakePushProvider) Send(ctx context.Context, device models.DeviceToken, message models.PushMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends++
	return f.err
}

func TestPushDispatcherDeliversToAllDevices(t *testing.T) {
	t.Parallel()

	devices := &fakeDeviceLister{
		tokens: []models.DeviceToken{
			{Token: "token-1", Platform: models.PlatformIOS},
			{Token: "token-2", Platform: models.PlatformAndroid},
		},
	}
	provider := &fakePushProvider{}
	dispatcher := NewPushDispatcher(devices, provider, 1, 4, nil)

	dispatcher.deliver(context.Background(), models.PushMessage{RecipientID: uuid.New(), Title: "hi"})

	if provider.sends != 2 {
		t.Errorf("sends = %d, want 2", provider.sends)
	}
}

func TestPushDispatcherDeactivatesStaleToken(t *testing.T) {
	t.Parallel()

	token := "stale-token"
	devices := &fakeDeviceLister{
		tokens: []models.DeviceToken{{Token: token, Platform: models.PlatformIOS}},
	}
	provider := &fakePushProvider{err: models.ErrTokenUnregistered}
	dispatcher := NewPushDispatcher(devices, provider, 1, 4, nil)

	dispatcher.deliver(context.Background(), models.PushMessage{RecipientID: uuid.New()})

	if len(devices.marked) != 1 || devices.marked[0] != token {
		t.Errorf("marked = %v, want [%s]", devices.marked, token)
	}
}

func TestPushDispatcherQueueBound(t *testing.T) {
	t.Parallel()

	dispatcher := NewPushDispatcher(&fakeDeviceLister{}, &fakePushProvider{}, 1, 1, nil)

	if err := dispatcher.Enqueue(context.Background(), models.PushMessage{RecipientID: uuid.New()}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := dispatcher.Enqueue(context.Background(), models.PushMessage{RecipientID: uuid.New()}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("second enqueue = %v, want ErrQueueFull", err)
	}
}

type fakeEmailProvider struct {
	mu    sync.Mutex
	sends int
}

func (f *fakeEmailProvider) Send(ctx context.Context, message models.EmailMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends++
	return nil
}

func TestEmailDispatcherWorker(t *testing.T) {
	t.Parallel()

	provider := &fakeEmailProvider{}
	dispatcher := NewEmailDispatcher(provider, 1, 4, nil)

	ctx, cancel := context.WithCancel(context.Background())
	dispatcher.Start(ctx)

	message := models.EmailMessage{
		RecipientID:    uuid.New(),
		RecipientEmail: "user@example.com",
		Subject:        "Hello",
		Body:           "World",
	}
	if err := dispatcher.Enqueue(ctx, message); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		provider.mu.Lock()
		sends := provider.sends
		provider.mu.Unlock()
		if sends == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	dispatcher.Wait()

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.sends != 1 {
		t.Errorf("sends = %d, want 1", provider.sends)
	}
}

func TestSignRS256(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	token, err := signRS256(key, map[string]any{"iss": "test", "exp": 12345})
	if err != nil {
		t.Fatalf("signRS256: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d", len(parts))
	}

	headerJSON, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var header map[string]string
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("header: %v", err)
	}
	if header["alg"] != "RS256" {
		t.Errorf("alg = %q", header["alg"])
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, hash[:], signature); err != nil {
		t.Errorf("RS256 signature did not verify: %v", err)
	}
}

func TestParseRSAPrivateKey(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	pemData := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))

	parsed, err := parseRSAPrivateKey(pemData)
	if err != nil {
		t.Fatalf("parseRSAPrivateKey: %v", err)
	}
	if parsed.N.Cmp(key.N) != 0 {
		t.Error("parsed key differs")
	}

	if _, err := parseRSAPrivateKey("not pem"); err == nil {
		t.Error("invalid PEM should fail")
	}
}

func TestSignES256(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	token, err := signES256(key, "key-id", map[string]any{"iss": "team-id"})
	if err != nil {
		t.Fatalf("signES256: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d", len(parts))
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))

	byteLen := (key.Curve.Params().BitSize + 7) / 8
	r := new(big.Int).SetBytes(signature[:byteLen])
	s := new(big.Int).SetBytes(signature[byteLen:])
	if !ecdsa.Verify(&key.PublicKey, hash[:], r, s) {
		t.Error("ES256 signature did not verify")
	}
}
