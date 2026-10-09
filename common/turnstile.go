package common

// TurnstileConfigured reports whether server-side verification has both
// required credentials. The secret is never returned or logged.
func TurnstileConfigured() bool {
	return TurnstileSiteKey != "" && TurnstileSecretKey != ""
}
