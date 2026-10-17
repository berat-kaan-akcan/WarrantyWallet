package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSMTP is a minimal plaintext SMTP server that records one message.
func fakeSMTP(t *testing.T) (port int, got chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		w("220 fake ESMTP")
		var transcript strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			transcript.WriteString(line)
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				w("250-fake")
				w("250 AUTH PLAIN")
			case strings.HasPrefix(cmd, "AUTH PLAIN"):
				w("235 ok")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				w("250 ok")
			case cmd == "DATA":
				w("354 go")
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					transcript.WriteString(l)
				}
				w("250 queued")
			case cmd == "QUIT":
				w("221 bye")
				got <- transcript.String()
				return
			default:
				w("250 ok")
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, got
}

func TestSMTPSenderDeliversVerification(t *testing.T) {
	port, got := fakeSMTP(t)
	s := SMTPSender{Host: "127.0.0.1", Port: port, Username: "u", Password: "p", From: "Warranty Wallet <noreply@ww.test>", Timeout: 5 * time.Second}
	if err := s.SendEmailVerification(context.Background(), "berat@example.com", "Berat", "https://api.ww.test/api/v1/auth/email/verify?token=abc"); err != nil {
		t.Fatal(err)
	}
	tr := <-got
	auth := base64.StdEncoding.EncodeToString([]byte("\x00u\x00p"))
	for _, want := range []string{"MAIL FROM:<noreply@ww.test>", "RCPT TO:<berat@example.com>", "AUTH PLAIN " + auth, "Subject: =?utf-8?q?", "charset=utf-8"} {
		if !strings.Contains(tr, want) {
			t.Errorf("transcript missing %q:\n%s", want, tr)
		}
	}
	// Body is base64; decode and check the link.
	body := tr[strings.Index(tr, "base64\r\n\r\n")+len("base64\r\n\r\n"):]
	body, _, _ = strings.Cut(body, "QUIT")
	dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(body), "\r\n", ""))
	if err != nil || !strings.Contains(string(dec), "token=abc") || !strings.Contains(string(dec), "Merhaba Berat") {
		t.Fatalf("body %q %v", dec, err)
	}
}

func TestBuildMessageRejectsHeaderInjection(t *testing.T) {
	if _, err := BuildMessage("a@b.c", "x@y.z\r\nBcc: evil@z.z", "s", "b", time.Now()); err == nil {
		t.Fatal("expected error")
	}
	if greetName("Ali\r\nBcc: x") != "Ali  Bcc: x" || greetName(" ") != "değerli kullanıcımız" {
		t.Fatal("greetName")
	}
}

func TestSMTPSenderErrors(t *testing.T) {
	s := SMTPSender{Host: "127.0.0.1", Port: 1, From: "bad", Timeout: time.Second}
	if err := s.SendPasswordReset(context.Background(), "a@b.c", "", "t"); err == nil || !strings.Contains(err.Error(), "SMTP_FROM") {
		t.Fatalf("from: %v", err)
	}
	s.From = "noreply@ww.test"
	if err := s.SendPasswordReset(context.Background(), "a@b.c", "", "t"); err == nil || !strings.Contains(err.Error(), "dial") {
		t.Fatalf("dial: %v", err)
	}
}
