package repositories

import "testing"

func TestReferralCodeGenerationAndNormalization(t *testing.T) {
	code, err := randomReferralCode(7)
	if err != nil || len(code) != 7 {
		t.Fatalf("code=%q err=%v", code, err)
	}
	if NormalizeReferralCode(code) != code {
		t.Errorf("generated code not normalized: %q", code)
	}
	if got := NormalizeReferralCode("  ab-c2 d9 "); got != "ABC2D9" {
		t.Errorf("normalize = %q", got)
	}
	if got := NormalizeReferralCode("O0I1"); got != "" {
		t.Errorf("ambiguous chars should be dropped, got %q", got)
	}
}
