package httpapi

import (
	"net/http"
	"strings"
	"unicode"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

func validPushToken(t string) bool {
	if len(t) < 20 || len(t) > 4096 {
		return false
	}
	return strings.IndexFunc(t, func(r rune) bool { return unicode.IsSpace(r) || r > unicode.MaxASCII }) < 0
}

// handleRegisterDevice stores the app's FCM token so reminders are pushed to this phone.
func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Platform string `json:"platform"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	fields := map[string]string{}
	if !validPushToken(body.Token) {
		fields["token"] = "must be an FCM registration token"
	}
	if body.Platform != "android" && body.Platform != "ios" {
		fields["platform"] = "must be android or ios"
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid device", fields)
		return
	}
	d := &domain.DeviceToken{UserID: userFrom(r.Context()).ID, Token: body.Token, Platform: body.Platform}
	if err := s.Store.SaveDeviceToken(r.Context(), d); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUnregisterDevice forgets a token of the current user (sign-out, notifications turned off).
func (s *Server) handleUnregisterDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteDeviceToken(r.Context(), userFrom(r.Context()).ID, r.PathValue("token")); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
