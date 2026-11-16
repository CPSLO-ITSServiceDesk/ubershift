package autoshift

import (
	"net/http/httptest"
	"testing"
)

func TestSupabaseServerKeyPrefersCurrentSecretKey(t *testing.T) {
	t.Setenv(ubershiftSupabaseKeyVar, "")
	t.Setenv(supabaseServiceRoleKeyVar, "legacy-service-role-key")
	t.Setenv(supabaseSecretKeyVar, "sb_secret_current-key")

	if got := supabaseServerKey(); got != "sb_secret_current-key" {
		t.Errorf("supabaseServerKey() = %q, want current secret key", got)
	}
}

func TestSupabaseServerKeyFallsBackToLegacyServiceRoleKey(t *testing.T) {
	t.Setenv(ubershiftSupabaseKeyVar, "")
	t.Setenv(supabaseSecretKeyVar, "")
	t.Setenv(supabaseServiceRoleKeyVar, "legacy-service-role-key")

	if got := supabaseServerKey(); got != "legacy-service-role-key" {
		t.Errorf("supabaseServerKey() = %q, want legacy service role key", got)
	}
}

func TestSupabaseServerKeyPrefersAutoShiftDedicatedSecret(t *testing.T) {
	t.Setenv(ubershiftSupabaseKeyVar, "sb_secret_autoshift-key")
	t.Setenv(supabaseSecretKeyVar, "sk_live_vercel-connector-key")
	t.Setenv(supabaseServiceRoleKeyVar, "legacy-service-role-key")

	if got := supabaseServerKey(); got != "sb_secret_autoshift-key" {
		t.Errorf("supabaseServerKey() = %q, want dedicated AutoShift secret", got)
	}
}

func TestSupabaseServerKeyIgnoresVercelConnectorCredential(t *testing.T) {
	t.Setenv(ubershiftSupabaseKeyVar, "")
	t.Setenv(supabaseSecretKeyVar, "sk_live_vercel-connector-key")
	t.Setenv(supabaseServiceRoleKeyVar, "legacy-service-role-key")

	if got := supabaseServerKey(); got != "legacy-service-role-key" {
		t.Errorf("supabaseServerKey() = %q, want legacy service role key", got)
	}
}

func TestSetSupabaseAuthHeadersUsesAPIKeyOnlyForCurrentKeys(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.test", nil)
	setSupabaseAuthHeaders(req, "sb_secret_current-key")

	if got := req.Header.Get("apikey"); got != "sb_secret_current-key" {
		t.Errorf("apikey = %q, want current secret key", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty", got)
	}
}

func TestSetSupabaseAuthHeadersSupportsLegacyServiceRoleKeys(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.test", nil)
	setSupabaseAuthHeaders(req, "legacy-service-role-key")

	if got := req.Header.Get("Authorization"); got != "Bearer legacy-service-role-key" {
		t.Errorf("Authorization = %q, want legacy bearer token", got)
	}
}
