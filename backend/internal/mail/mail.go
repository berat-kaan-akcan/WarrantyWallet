// Package mail sends transactional e-mails. LogSender is used when SMTP is not configured
// (development); SMTPSender delivers real mail. Handlers only see the Sender interface.
package mail

import (
	"context"
	"log/slog"
	"strings"
)

// Sender delivers account e-mails.
type Sender interface {
	SendPasswordReset(ctx context.Context, to, name, token string) error
	SendEmailVerification(ctx context.Context, to, name, link string) error
}

// LogSender records that a mail would have been sent. It never logs the token or link.
type LogSender struct{ Logger *slog.Logger }

// SendPasswordReset implements Sender.
func (l LogSender) SendPasswordReset(_ context.Context, to, _ string, _ string) error {
	if l.Logger != nil {
		l.Logger.Info("password reset e-mail (not sent: no SMTP configured)", "to", MaskEmail(to))
	}
	return nil
}

// SendEmailVerification implements Sender.
func (l LogSender) SendEmailVerification(_ context.Context, to, _ string, _ string) error {
	if l.Logger != nil {
		l.Logger.Info("verification e-mail (not sent: no SMTP configured)", "to", MaskEmail(to))
	}
	return nil
}

// MaskEmail hides most of the local part: "berat@example.com" → "b***@example.com".
func MaskEmail(e string) string {
	local, domain, ok := strings.Cut(e, "@")
	if !ok || local == "" {
		return "***"
	}
	return local[:1] + "***@" + domain
}
