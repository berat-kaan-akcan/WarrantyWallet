package push

import (
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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store/memory"
)

// fakeGoogle verifies the service-account JWT and accepts FCM sends.
type fakeGoogle struct {
	t        *testing.T
	pub      *rsa.PublicKey
	mu       sync.Mutex
	oauth    int
	sent     []map[string]any
	reject   map[string]int // token → HTTP status
	tokenURL string
}

func (g *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case r.URL.Path == "/token":
		_ = r.ParseForm()
		parts := strings.Split(r.PostForm.Get("assertion"), ".")
		if len(parts) != 3 || r.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			http.Error(w, `{"error":"invalid_grant"}`, 400)
			return
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if rsa.VerifyPKCS1v15(g.pub, crypto.SHA256, sum[:], sig) != nil {
			http.Error(w, `{"error":"invalid_grant"}`, 400)
			return
		}
		var claims map[string]any
		raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
		_ = json.Unmarshal(raw, &claims)
		if claims["iss"] != "svc@proj.iam.gserviceaccount.com" || claims["scope"] != fcmScope || claims["aud"] != g.tokenURL {
			g.t.Errorf("claims %v", claims)
		}
		g.oauth++
		_, _ = w.Write([]byte(`{"access_token":"ya29.test","expires_in":3600}`))
	case r.URL.Path == "/v1/projects/proj/messages:send":
		if r.Header.Get("Authorization") != "Bearer ya29.test" {
			http.Error(w, `{"error":{"status":"UNAUTHENTICATED"}}`, 401)
			return
		}
		var body map[string]map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		tok, _ := body["message"]["token"].(string)
		if code := g.reject[tok]; code != 0 {
			status := "UNAVAILABLE"
			if code == 404 {
				status = "NOT_FOUND\", \"details\": [{\"errorCode\": \"UNREGISTERED\"}]"
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"error":{"status":"` + status + `"}}`))
			return
		}
		g.sent = append(g.sent, body["message"])
		_, _ = w.Write([]byte(`{"name":"projects/proj/messages/1"}`))
	default:
		http.NotFound(w, r)
	}
}

func setup(t *testing.T) (*FCM, *fakeGoogle) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	g := &fakeGoogle{t: t, pub: &key.PublicKey, reject: map[string]int{}}
	srv := httptest.NewServer(g)
	t.Cleanup(srv.Close)
	g.tokenURL = srv.URL + "/token"
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	cred, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "proj", "client_email": "svc@proj.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": g.tokenURL,
	})
	path := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(path, cred, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadFCM(path)
	if err != nil {
		t.Fatal(err)
	}
	f.BaseURL = srv.URL
	return f, g
}

func TestFCMSendCachesOAuthToken(t *testing.T) {
	f, g := setup(t)
	ctx := context.Background()
	for _, tok := range []string{"device-a", "device-b"} {
		if err := f.Send(ctx, tok, Message{Title: "T", Body: "B", Data: map[string]string{"product_id": "p1"}}); err != nil {
			t.Fatal(err)
		}
	}
	if g.oauth != 1 || len(g.sent) != 2 {
		t.Fatalf("oauth %d sent %d", g.oauth, len(g.sent))
	}
	n := g.sent[0]["notification"].(map[string]any)
	if n["title"] != "T" || n["body"] != "B" || g.sent[0]["data"].(map[string]any)["product_id"] != "p1" {
		t.Fatalf("payload %v", g.sent[0])
	}
	// token expiry → a new OAuth exchange
	f.Now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	_ = f.Send(ctx, "device-a", Message{})
	if g.oauth != 2 {
		t.Fatalf("expired token reused, oauth %d", g.oauth)
	}
	g.reject["gone"] = 404
	if err := f.Send(ctx, "gone", Message{}); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("404 → %v", err)
	}
	g.reject["busy"] = 503
	if err := f.Send(ctx, "busy", Message{}); err == nil || errors.Is(err, ErrUnregistered) || !strings.Contains(err.Error(), "UNAVAILABLE") {
		t.Fatalf("503 → %v", err)
	}
}

func TestLoadFCMRejectsBadCredentials(t *testing.T) {
	dir := t.TempDir()
	for i, body := range []string{`nope`, `{"project_id":"p"}`, `{"project_id":"p","client_email":"e","private_key":"x"}`} {
		p := filepath.Join(dir, string(rune('a'+i)))
		_ = os.WriteFile(p, []byte(body), 0o600)
		if _, err := LoadFCM(p); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	if _, err := LoadFCM(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file accepted")
	}
}

type fakeSender struct {
	fail map[string]error
	got  []string
}

func (s *fakeSender) Send(_ context.Context, token string, m Message) error {
	s.got = append(s.got, token+":"+m.Body)
	return s.fail[token]
}

func TestNotifierDeliver(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	u := &domain.User{Email: "n@x.io", PasswordHash: "h"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{"ok", "gone", "busy"} {
		_ = st.SaveDeviceToken(ctx, &domain.DeviceToken{UserID: u.ID, Token: tok, Platform: "android"})
	}
	s := &fakeSender{fail: map[string]error{"gone": ErrUnregistered, "busy": errors.New("503")}}
	n := &Notifier{Store: st, Sender: s}
	note := domain.Notification{ID: "n1", UserID: u.ID, ProductID: "p1", Type: "warranty_30", Message: "30 gün kaldı."}
	if err := n.Deliver(ctx, note); err != nil {
		t.Fatalf("one device succeeded, got %v", err)
	}
	if len(s.got) != 3 || !strings.HasSuffix(s.got[0], ":30 gün kaldı.") {
		t.Fatalf("sent %v", s.got)
	}
	left, _ := st.ListDeviceTokens(ctx, u.ID)
	if len(left) != 2 {
		t.Fatalf("unregistered token kept: %+v", left)
	}
	_ = st.DeleteDeviceToken(ctx, "", "ok")
	if err := n.Deliver(ctx, note); err == nil {
		t.Fatal("all devices failing temporarily must be retried")
	}
	_ = st.DeleteDeviceToken(ctx, "", "busy")
	if err := n.Deliver(ctx, note); err != nil {
		t.Fatalf("no devices → nothing to do, got %v", err)
	}
}
