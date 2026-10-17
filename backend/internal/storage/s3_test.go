package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// AWS's published SigV4 example ("GET Object" in the S3 signature documentation).
func TestS3SignMatchesAWSExample(t *testing.T) {
	s, err := NewS3(S3{
		Endpoint:  "https://s3.amazonaws.com",
		Bucket:    "examplebucket",
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
		SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Now:       func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := s.objectURL("test.txt")
	if u != "https://examplebucket.s3.amazonaws.com/test.txt" {
		t.Fatalf("virtual-hosted url %s", u)
	}
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Range", "bytes=0-9")
	s.sign(req, nil)
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("authorization\n got %s\nwant %s", got, want)
	}
}

// fakeS3 is a minimal path-style bucket that checks the payload hash header.
type fakeS3 struct {
	mu   sync.Mutex
	objs map[string][]byte
	auth []string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	if !strings.HasPrefix(r.URL.Path, "/bucket/") {
		http.Error(w, "<Error><Code>NoSuchBucket</Code></Error>", http.StatusNotFound)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	switch r.Method {
	case "PUT":
		b, _ := io.ReadAll(r.Body)
		h := sha256.Sum256(b)
		if r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(h[:]) {
			http.Error(w, "<Error><Code>XAmzContentSHA256Mismatch</Code></Error>", http.StatusBadRequest)
			return
		}
		f.objs[key] = b
	case "GET":
		b, ok := f.objs[key]
		if !ok {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		_, _ = w.Write(b)
	case "DELETE":
		delete(f.objs, key)
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestS3RoundTrip(t *testing.T) {
	ctx := context.Background()
	fake := &fakeS3{objs: map[string][]byte{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	s, err := NewS3(S3{Endpoint: srv.URL + "/", Bucket: "bucket", AccessKey: "ak", SecretKey: "sk", PathStyle: true, MaxSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.Put(ctx, "u1/p1/doc.jpg", strings.NewReader("hello")); err != nil || n != 5 {
		t.Fatalf("put %d %v", n, err)
	}
	rc, err := s.Open(ctx, "u1/p1/doc.jpg")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello" {
		t.Fatalf("got %q", b)
	}
	if !strings.HasPrefix(fake.auth[0], "AWS4-HMAC-SHA256 Credential=ak/") || strings.Contains(fake.auth[0], "sk") {
		t.Fatalf("auth header %q", fake.auth[0])
	}
	if _, err := s.Put(ctx, "big", strings.NewReader("01234567890")); err == nil {
		t.Fatal("oversized put accepted")
	}
	if err := s.Delete(ctx, "u1/p1/doc.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, "u1/p1/doc.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete %v", err)
	}
	if err := s.Delete(ctx, "u1/p1/doc.jpg"); err != nil {
		t.Fatalf("delete missing %v", err)
	}
	if _, err := s.Put(ctx, "../etc/passwd", strings.NewReader("x")); err == nil {
		t.Fatal("bad key accepted")
	}
	s.Bucket = "other"
	_, err = s.Put(ctx, "a", strings.NewReader("x"))
	if err == nil || !strings.Contains(err.Error(), "NoSuchBucket") {
		t.Fatalf("error should name the S3 code: %v", err)
	}
}

func TestNewS3Validation(t *testing.T) {
	for _, c := range []S3{
		{Endpoint: "ftp://x", Bucket: "b", AccessKey: "a", SecretKey: "s"},
		{Endpoint: "http://minio:9000", AccessKey: "a", SecretKey: "s"},
		{Endpoint: "http://minio:9000", Bucket: "b"},
	} {
		if _, err := NewS3(c); err == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
}
