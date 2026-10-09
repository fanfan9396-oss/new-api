package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNotifyAbuseSignalSkipsLowRiskAndSendsRedactedReview(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()
	withSMTPSettings(t)
	SMTPServer, SMTPPort = server.host, server.port
	SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPInsecureSkipVerify = false, true, true
	SMTPAccount, SMTPFrom, SMTPToken = "sender@example.com", "sender@example.com", "secret"
	previousEnabled, previousRecipient := AbuseAlertEnabled, AbuseAlertRecipient
	AbuseAlertEnabled, AbuseAlertRecipient = true, "ops@example.com"
	t.Cleanup(func() { AbuseAlertEnabled, AbuseAlertRecipient = previousEnabled, previousRecipient })

	NotifyAbuseSignal("event-low", "abuse.multi_ip", 2, `{"model":"safe"}`)
	select {
	case <-server.messages:
		t.Fatal("low risk signal must not send an alert")
	case <-time.After(50 * time.Millisecond):
	}

	NotifyAbuseSignal("event-high", "abuse.provider_error", 4, `{"model":"<redacted>"}`)
	select {
	case message := <-server.messages:
		require.Contains(t, message, "event-high")
		require.Contains(t, message, "manual review")
		require.Contains(t, message, "&lt;redacted&gt;")
	case <-time.After(2 * time.Second):
		t.Fatal("expected abuse alert delivery")
	}
}
