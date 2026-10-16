package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLocal(t *testing.T) {
	ctx := context.Background()
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n, err := l.Put(ctx, "u1/p1/doc.jpg", strings.NewReader("hello"))
	if err != nil || n != 5 {
		t.Fatalf("put %d %v", n, err)
	}
	rc, err := l.Open(ctx, "u1/p1/doc.jpg")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello" {
		t.Fatalf("got %q", b)
	}
	if err := l.Delete(ctx, "u1/p1/doc.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Open(ctx, "u1/p1/doc.jpg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	if err := l.Delete(ctx, "u1/p1/doc.jpg"); err != nil {
		t.Fatal("delete of missing key should be nil")
	}
	for _, bad := range []string{"../etc/passwd", "/abs", "a/../../b", ""} {
		if _, err := l.Put(ctx, bad, strings.NewReader("x")); err == nil {
			t.Fatalf("accepted bad key %q", bad)
		}
	}
}
