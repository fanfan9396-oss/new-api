# Content audit shadow mode

New API can send a sampled copy of the normalized request text to an
OpenAI-compatible audit endpoint. This path is asynchronous and advisory:
an audit result can create an `abuse.content_audit` review record, but it
cannot reject a request, freeze a user, revoke a token, change a wallet, or
send a user-behavior email.

The feature is disabled unless all of these variables are set:

```text
CONTENT_AUDIT_ENABLED=false
CONTENT_AUDIT_ENDPOINT=https://audit.example/v1/chat/completions
CONTENT_AUDIT_API_KEY=<secret kept in the server secret store>
CONTENT_AUDIT_MODEL=<approved audit model>
CONTENT_AUDIT_TIMEOUT_MS=2000
CONTENT_AUDIT_SAMPLE_RATE=0.05
```

The administrator can edit the non-secret values from **System Settings ->
Request Policies -> Request checks**. Persisted options take precedence over
the corresponding environment fallback. The API key is intentionally absent
from that page and from the `Option` table; it must remain in the server secret
environment as `CONTENT_AUDIT_API_KEY`.

`CONTENT_AUDIT_ENDPOINT` must be the complete chat-completions URL. The
adapter sends a fixed system policy, wraps untrusted text in
`<user_input>`, requires the documented JSON decision, and accepts only the
categories and `allow`/`review` actions in the adapter. Invalid JSON, HTTP
errors, timeouts, and queue saturation are dropped from the decision path and
are visible only in the administrator audit summary counters.

The administrator audit page exposes the queue at **Audit Logs -> Abuse
Reviews**. Reviewers can filter by status or signal, inspect redacted evidence,
and close a record as `resolved` or `false_positive`. The summary is an
in-process operational view; its model health counters reset after a process
restart, while review records remain in the `abuse_reviews` table.

The request policy page is an operator configuration surface, not a blocking
policy editor. The immutable safety boundary is always prepended to any
additional instructions, the reserved blocking threshold is informational, and
the implementation only creates review records after the review confidence
threshold is met.

Before enabling this in any staging environment:

1. Use a dedicated low-cost audit endpoint and a key stored outside Git.
2. Keep `CONTENT_AUDIT_SAMPLE_RATE` low and keep production blocking disabled.
3. Run a fixed, redacted positive/negative sample set and record false
   positives, false negatives, P95 latency, and spend.
4. Confirm that ordinary self-owned code, deployment, and defensive testing
   requests remain `allow` or `review` for human inspection rather than being
   synchronously blocked.

The feature must remain disabled in production until those measurements and a
separate operator decision are recorded.
