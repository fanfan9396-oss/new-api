package common

import "testing"

func TestTurnstileConfiguredRequiresSiteAndSecret(t *testing.T) {
	previousSite, previousSecret := TurnstileSiteKey, TurnstileSecretKey
	t.Cleanup(func() { TurnstileSiteKey, TurnstileSecretKey = previousSite, previousSecret })
	TurnstileSiteKey, TurnstileSecretKey = "site", ""
	if TurnstileConfigured() {
		t.Fatal("Turnstile must not be configured without a server secret")
	}
	TurnstileSecretKey = "secret"
	if !TurnstileConfigured() {
		t.Fatal("Turnstile should be configured when site and secret are present")
	}
}
