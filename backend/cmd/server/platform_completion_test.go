package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestArgon2PasswordAndLegacyUpgrade(t *testing.T) {
	hash, err := hashPassword("StrongPass123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || !verifyPassword("StrongPass123", hash) {
		t.Fatalf("new password hash is not a valid Argon2id hash: %q", hash)
	}
	if verifyPassword("wrong-password", hash) || passwordNeedsUpgrade(hash) {
		t.Fatal("Argon2id verification or upgrade detection is incorrect")
	}
	legacySalt := []byte("0123456789abcdef")
	legacy := hex.EncodeToString(legacySalt) + ":" + hex.EncodeToString(pbkdf2Key([]byte("StrongPass123"), legacySalt, 120000, 32, sha256.New))
	if !verifyPassword("StrongPass123", legacy) || !passwordNeedsUpgrade(legacy) {
		t.Fatal("legacy PBKDF2 compatibility is broken")
	}
}

func TestRequestLimiterWindow(t *testing.T) {
	limiter := newRequestLimiter()
	if !limiter.allow("registration", 2, time.Hour) || !limiter.allow("registration", 2, time.Hour) {
		t.Fatal("requests below the limit should be allowed")
	}
	if limiter.allow("registration", 2, time.Hour) {
		t.Fatal("request above the limit should be rejected")
	}
	item := limiter.windows["registration"]
	item.StartedAt = time.Now().Add(-2 * time.Hour)
	limiter.windows["registration"] = item
	if !limiter.allow("registration", 2, time.Hour) {
		t.Fatal("expired request window should reset")
	}
}

func TestCheckinCursorRoundTrip(t *testing.T) {
	encoded := encodeCheckinCursor("2026-08-26", 987)
	date, id, ok := parseCheckinCursor(encoded)
	if !ok || date != "2026-08-26" || id != 987 {
		t.Fatalf("unexpected cursor result: %q %d %v", date, id, ok)
	}
	for _, value := range []string{"", "invalid", encodeCheckinCursor("bad", 1)} {
		if _, _, ok := parseCheckinCursor(value); ok {
			t.Fatalf("invalid cursor %q accepted", value)
		}
	}
}

func TestGroupOptionsDefaultsAndBounds(t *testing.T) {
	defaults := groupOptionsFromSettings(nil)
	if defaults.RetroDays != 30 || !defaults.ShowGroupSummary || defaults.AllowMemberRanking {
		t.Fatalf("unsafe defaults: %+v", defaults)
	}
	options := groupOptionsFromSettings(map[string]any{"group_options": map[string]any{
		"retro_days": float64(999), "show_reflections": true, "allow_member_ranking": true,
	}})
	if options.RetroDays != 90 || !options.ShowReflections || !options.AllowMemberRanking {
		t.Fatalf("group options were not parsed safely: %+v", options)
	}
}

func TestRandomTemporaryPasswordMeetsPolicy(t *testing.T) {
	for i := 0; i < 20; i++ {
		password := randomPassword(12)
		if !validRegistrationPassword(password) {
			t.Fatalf("temporary password does not meet policy: %q", password)
		}
	}
}
