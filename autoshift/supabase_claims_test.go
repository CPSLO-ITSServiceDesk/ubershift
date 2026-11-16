package autoshift

import "testing"

func TestSupabaseServerKeyPrefersCurrentSecretKey(t *testing.T) {
	t.Setenv(supabaseServiceRoleKeyVar, "legacy-service-role-key")
	t.Setenv(supabaseSecretKeyVar, "sb_secret_current-key")

	if got := supabaseServerKey(); got != "sb_secret_current-key" {
		t.Errorf("supabaseServerKey() = %q, want current secret key", got)
	}
}

func TestSupabaseServerKeyFallsBackToLegacyServiceRoleKey(t *testing.T) {
	t.Setenv(supabaseSecretKeyVar, "")
	t.Setenv(supabaseServiceRoleKeyVar, "legacy-service-role-key")

	if got := supabaseServerKey(); got != "legacy-service-role-key" {
		t.Errorf("supabaseServerKey() = %q, want legacy service role key", got)
	}
}
