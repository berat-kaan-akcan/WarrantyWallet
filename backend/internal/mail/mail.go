// Package mail sends transactional e-mails. V1 ships only a logging sender (no SMTP yet):
// the password reset flow is complete on the API side and an SMTP/SES sender can be plugged
// in behind the Sender interface without touching the handlers.
package mail

import (
	"context"
	"log/slog"
	"strings"
)

// Sender delivers account e-mails.
type Sender interface {
	SendPasswordReset(ctx context.Context, to, name, token string) error
}

// LogSender records that a mail would have been sent. It never logs the token itself.
type LogSender struct{ Logger *slog.Logger }

// SendPasswordReset implements Sender.
func (l LogSender) SendPasswordReset(_ context.Context, to, _ string, _ string) error {
	if l.Logger != nil {
		l.Logger.Info("password reset e-mail (not sent: no SMTP configured)", "to", MaskEmail(to))
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
