package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SMTPSender delivers mail through an SMTP relay (STARTTLS on 587, or implicit TLS on 465).
type SMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // "Warranty Wallet <noreply@example.com>"
	Timeout  time.Duration
}

// SendPasswordReset implements Sender.
func (s SMTPSender) SendPasswordReset(ctx context.Context, to, name, token string) error {
	body := fmt.Sprintf("Merhaba %s,\r\n\r\nWarranty Wallet hesabınız için şifre sıfırlama isteği aldık.\r\n"+
		"Uygulamada \"Şifremi unuttum\" ekranına aşağıdaki kodu girin (1 saat geçerlidir):\r\n\r\n%s\r\n\r\n"+
		"Bu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz; şifreniz değişmez.\r\n", greetName(name), token)
	return s.send(ctx, to, "Warranty Wallet şifre sıfırlama kodu", body)
}

// SendEmailVerification implements Sender.
func (s SMTPSender) SendEmailVerification(ctx context.Context, to, name, link string) error {
	body := fmt.Sprintf("Merhaba %s,\r\n\r\nWarranty Wallet'a hoş geldiniz! E-posta adresinizi doğrulamak için "+
		"aşağıdaki bağlantıyı açın (24 saat geçerlidir):\r\n\r\n%s\r\n\r\n"+
		"Hesabı siz oluşturmadıysanız bu e-postayı yok sayabilirsiniz.\r\n", greetName(name), link)
	return s.send(ctx, to, "Warranty Wallet e-posta doğrulama", body)
}

func greetName(n string) string {
	n = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, n))
	if n == "" {
		return "değerli kullanıcımız"
	}
	return n
}

// BuildMessage renders a UTF-8 plain-text message. Header values are checked for CR/LF so user
// data can never inject headers.
func BuildMessage(from, to, subject, body string, now time.Time) ([]byte, error) {
	for _, v := range []string{from, to, subject} {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errors.New("mail: header contains a line break")
		}
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", now.Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(body))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.Bytes(), nil
}

func (s SMTPSender) send(ctx context.Context, to, subject, body string) error {
	fromAddr, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("mail: invalid SMTP_FROM: %w", err)
	}
	toAddr, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("mail: invalid recipient: %w", err)
	}
	msg, err := BuildMessage(fromAddr.String(), toAddr.String(), subject, body, time.Now())
	if err != nil {
		return err
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	addr := net.JoinHostPort(s.Host, fmt.Sprint(s.Port))
	d := net.Dialer{Timeout: timeout}
	var conn net.Conn
	if s.Port == 465 {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: dial: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("mail: hello: %w", err)
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok && s.Port != 465 {
		if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if s.Username != "" {
		// PlainAuth refuses to send credentials without TLS (except to localhost).
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := c.Mail(fromAddr.Address); err != nil {
		return fmt.Errorf("mail: from: %w", err)
	}
	if err := c.Rcpt(toAddr.Address); err != nil {
		return fmt.Errorf("mail: rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return c.Quit()
}
