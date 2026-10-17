package httpapi

import (
	"context"
	"testing"
)

func TestDeviceRegistration(t *testing.T) {
	h := newHarness(t)
	tok := h.register("dev@example.com", "D")
	fcm := "fGx1:APA91bH-example_token_value"
	if r := h.do("POST", "/api/v1/devices", tok, map[string]string{"token": "short", "platform": "web"}); r.Status != 422 {
		t.Fatalf("validation %d", r.Status)
	} else if f := r.json(t)["error"].(map[string]any)["fields"].(map[string]any); f["token"] == nil || f["platform"] == nil {
		t.Fatalf("fields %v", f)
	}
	if r := h.do("POST", "/api/v1/devices", "", map[string]string{"token": fcm, "platform": "android"}); r.Status != 401 {
		t.Fatalf("unauthenticated %d", r.Status)
	}
	for i := 0; i < 2; i++ { // idempotent
		if r := h.do("POST", "/api/v1/devices", tok, map[string]string{"token": fcm, "platform": "android"}); r.Status != 204 {
			t.Fatalf("register %d %s", r.Status, r.Body)
		}
	}
	me, _ := h.api.Store.GetUserByEmail(context.Background(), "dev@example.com")
	if list, _ := h.api.Store.ListDeviceTokens(context.Background(), me.ID); len(list) != 1 || list[0].Platform != "android" {
		t.Fatalf("stored %+v", list)
	}
	other := h.register("other@example.com", "O")
	if r := h.do("DELETE", "/api/v1/devices/"+fcm, other, nil); r.Status != 204 {
		t.Fatalf("delete foreign %d", r.Status)
	}
	if list, _ := h.api.Store.ListDeviceTokens(context.Background(), me.ID); len(list) != 1 {
		t.Fatal("another user removed my device")
	}
	if r := h.do("DELETE", "/api/v1/devices/"+fcm, tok, nil); r.Status != 204 {
		t.Fatalf("delete %d", r.Status)
	}
	if list, _ := h.api.Store.ListDeviceTokens(context.Background(), me.ID); len(list) != 0 {
		t.Fatalf("not deleted %+v", list)
	}
}
