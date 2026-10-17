// Package push delivers warranty reminders to phones through Firebase Cloud Messaging (HTTP v1).
// It uses only the standard library: the service-account JWT is signed with RS256 and exchanged
// for a short-lived OAuth access token, which is cached until shortly before it expires.
package push

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
)

// Message is one push notification.
type Message struct {
	Title string
	Body  string
	Data  map[string]string
}

// Sender delivers a message to one device token.
type Sender interface {
	Send(ctx context.Context, token string, m Message) error
}

// ErrUnregistered means the token is no longer valid (app uninstalled, token rotated) and should be dropped.
var ErrUnregistered = errors.New("push token is no longer registered")

const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

// FCM sends through the Firebase Cloud Messaging HTTP v1 API.
type FCM struct {
	ProjectID   string
	ClientEmail string
	Key         *rsa.PrivateKey
	TokenURL    string // default https://oauth2.googleapis.com/token
	BaseURL     string // default https://fcm.googleapis.com
	Client      *http.Client
	Now         func() time.Time

	mu      sync.Mutex
	access  string
	expires time.Time
}

// LoadFCM reads a Firebase service-account JSON file (Project settings → Service accounts).
// The file is a secret: keep it outside the repository and pass only its path.
func LoadFCM(path string) (*FCM, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read FCM credentials: %w", err)
	}
	var sa struct {
		ProjectID   string `json:"project_id"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, errors.New("FCM credentials are not valid JSON")
	}
	if sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("FCM credentials need project_id, client_email and private_key")
	}
	key, err := parseKey(sa.PrivateKey)
	if err != nil {
		return nil, err
	}
	return &FCM{ProjectID: sa.ProjectID, ClientEmail: sa.ClientEmail, Key: key, TokenURL: sa.TokenURI}, nil
}

func parseKey(p string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(p))
	if block == nil {
		return nil, errors.New("FCM private_key is not PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, errors.New("FCM private_key is not RSA")
	}
	k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("FCM private_key cannot be parsed")
	}
	return k, nil
}

func (f *FCM) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *FCM) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// assertion builds the signed service-account JWT.
func (f *FCM) assertion(aud string) (string, error) {
	now := f.now()
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss": f.ClientEmail, "scope": fcmScope, "aud": aud,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	unsigned := b64(head) + "." + b64(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.Key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + b64(sig), nil
}

// accessToken returns a cached OAuth token, refreshing it a minute before expiry.
func (f *FCM) accessToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.access != "" && f.now().Before(f.expires.Add(-time.Minute)) {
		return f.access, nil
	}
	tokenURL := f.TokenURL
	if tokenURL == "" {
		tokenURL = "https://oauth2.googleapis.com/token"
	}
	jwt, err := f.assertion(tokenURL)
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {jwt}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := f.client().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&body)
	if res.StatusCode != http.StatusOK || body.AccessToken == "" {
		return "", fmt.Errorf("fcm oauth: HTTP %d %s", res.StatusCode, body.Error)
	}
	f.access = body.AccessToken
	f.expires = f.now().Add(time.Duration(body.ExpiresIn) * time.Second)
	return f.access, nil
}

// Send delivers one notification. ErrUnregistered tells the caller to forget the token.
func (f *FCM) Send(ctx context.Context, token string, m Message) error {
	access, err := f.accessToken(ctx)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"message": map[string]any{
		"token":        token,
		"notification": map[string]string{"title": m.Title, "body": m.Body},
		"data":         m.Data,
		"android":      map[string]any{"priority": "high", "notification": map[string]string{"channel_id": "warranty_reminders"}},
	}})
	base := f.BaseURL
	if base == "" {
		base = "https://fcm.googleapis.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/v1/projects/"+url.PathEscape(f.ProjectID)+"/messages:send", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	res, err := f.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
	if res.StatusCode == http.StatusNotFound || bytes.Contains(b, []byte("UNREGISTERED")) {
		return ErrUnregistered
	}
	if res.StatusCode == http.StatusUnauthorized {
		f.mu.Lock()
		f.access = "" // force a new OAuth token next time
		f.mu.Unlock()
	}
	var e struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &e)
	return fmt.Errorf("fcm send: HTTP %d %s", res.StatusCode, e.Error.Status)
}

// Notifier fans a reminder out to all of the user's devices. Its Deliver method plugs into
// notify.Worker.Send.
type Notifier struct {
	Store  store.Store
	Sender Sender
	Logger *slog.Logger
}

// Title is the push title for warranty reminders.
const Title = "Garanti hatırlatması"

// Deliver sends n to every registered device of its user. Invalid tokens are removed.
// It fails (so the reminder is retried on the next run) only when every device failed
// with a temporary error; a user without devices still has the in-app notification list.
func (p *Notifier) Deliver(ctx context.Context, n domain.Notification) error {
	devices, err := p.Store.ListDeviceTokens(ctx, n.UserID)
	if err != nil {
		return err
	}
	msg := Message{Title: Title, Body: n.Message, Data: map[string]string{
		"type": string(n.Type), "product_id": n.ProductID, "notification_id": n.ID,
	}}
	var lastErr error
	ok := 0
	for _, d := range devices {
		err := p.Sender.Send(ctx, d.Token, msg)
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrUnregistered):
			if derr := p.Store.DeleteDeviceToken(ctx, "", d.Token); derr != nil && p.Logger != nil {
				p.Logger.Warn("drop push token", "err", derr)
			}
		default:
			lastErr = err
		}
	}
	if ok == 0 && lastErr != nil {
		return lastErr
	}
	return nil
}
