package setting

import "testing"

func TestValidateModelRequestConcurrency(t *testing.T) {
	if err := ValidateModelRequestConcurrencyPerUser("5"); err != nil {
		t.Fatalf("expected valid per-user concurrency: %v", err)
	}
	if err := ValidateModelRequestConcurrencyGlobal("32"); err != nil {
		t.Fatalf("expected valid global concurrency: %v", err)
	}
	for _, value := range []string{"0", "-1", "100001", "not-a-number"} {
		if err := ValidateModelRequestConcurrencyPerUser(value); err == nil {
			t.Fatalf("expected invalid concurrency %q", value)
		}
	}
}
