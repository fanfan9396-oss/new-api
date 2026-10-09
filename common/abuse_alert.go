package common

import (
	"fmt"
	"html"
)

// NotifyAbuseSignal sends a redacted operator notification when explicitly
// enabled. The default is disabled so shadow signals cannot unexpectedly send
// mail or create an automatic enforcement path.
func NotifyAbuseSignal(eventID, action string, score int, evidence string) {
	if !AbuseAlertEnabled || AbuseAlertRecipient == "" || score < 3 {
		return
	}
	body := fmt.Sprintf("Abuse signal pending manual review\nEvent: %s\nAction: %s\nRisk score: %d\nDisposition: review\nRedacted evidence: %s\n", eventID, action, score, evidence)
	_ = SendEmail("[relay abuse] pending manual review", AbuseAlertRecipient, "<pre>"+html.EscapeString(body)+"</pre>")
}
