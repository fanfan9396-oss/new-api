package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEvaluateContentAuditParsesStrictShadowResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"flagged\":true,\"category\":\"cyber_abuse\",\"confidence\":0.91,\"reason_code\":\"third_party_attack\",\"action\":\"review\"}"}}]}`))
	}))
	defer server.Close()
	result, err := EvaluateContentAudit(context.Background(), ContentAuditConfig{
		Enabled: true, Endpoint: server.URL, APIKey: "test-key", Model: "audit-model", Timeout: time.Second,
	}, "audit this")
	require.NoError(t, err)
	require.True(t, result.Flagged)
	require.Equal(t, "cyber_abuse", result.Category)
	require.Equal(t, "review", result.Action)
}

func TestEvaluateContentAuditRejectsInvalidDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"flagged\":true,\"category\":\"unknown\",\"confidence\":0.9,\"reason_code\":\"x\",\"action\":\"block\"}"}}]}`))
	}))
	defer server.Close()
	_, err := EvaluateContentAudit(context.Background(), ContentAuditConfig{
		Enabled: true, Endpoint: server.URL, APIKey: "test-key", Model: "audit-model", Timeout: time.Second,
	}, "audit this")
	require.Error(t, err)
}

func TestEvaluateContentAuditDisabledDoesNotCallEndpoint(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer server.Close()
	_, err := EvaluateContentAudit(context.Background(), ContentAuditConfig{Endpoint: server.URL}, "audit this")
	require.ErrorIs(t, err, ErrContentAuditDisabled)
	require.False(t, called)
}

func TestEvaluateContentAuditTruncatesUnicodeByRune(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		var envelope struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request body: %v", err)
			return
		}
		if len(envelope.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(envelope.Messages))
			return
		}
		content := strings.TrimPrefix(envelope.Messages[1].Content, "<user_input>\n")
		content = strings.TrimSuffix(content, "\n</user_input>")
		if got := utf8.RuneCountInString(content); got != 12000 {
			t.Errorf("expected 12000 runes, got %d", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"flagged\":false,\"category\":\"none\",\"confidence\":0,\"reason_code\":\"\",\"action\":\"allow\"}"}}]}`))
	}))
	defer server.Close()
	result, err := EvaluateContentAudit(context.Background(), ContentAuditConfig{
		Enabled: true, Endpoint: server.URL, APIKey: "test-key", Model: "audit-model", Timeout: time.Second,
	}, strings.Repeat("审", 13000))
	require.NoError(t, err)
	require.False(t, result.Flagged)
}

func TestEvaluateContentAuditTimeoutFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()
	_, err := EvaluateContentAudit(context.Background(), ContentAuditConfig{
		Enabled: true, Endpoint: server.URL, APIKey: "test-key", Model: "audit-model", Timeout: 5 * time.Millisecond,
	}, "timeout")
	require.Error(t, err)
}

func TestEvaluateContentAuditRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 70*1024)))
	}))
	defer server.Close()
	_, err := EvaluateContentAudit(context.Background(), ContentAuditConfig{
		Enabled: true, Endpoint: server.URL, APIKey: "test-key", Model: "audit-model", Timeout: time.Second,
	}, "oversized")
	require.Error(t, err)
}

func TestFetchContentAuditModelsDiscoversAndDeduplicatesIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"audit-small"},{"id":"audit-small"},{"id":"audit-large"}]}`))
	}))
	defer server.Close()
	t.Setenv("CONTENT_AUDIT_API_KEY", "test-key")
	models, err := FetchContentAuditModels(context.Background(), server.URL+"/v1/chat/completions")
	require.NoError(t, err)
	require.Equal(t, []string{"audit-small", "audit-large"}, models)
}

func TestEvaluateContentAuditAppendsCompletionsPathForV1Endpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"flagged\":false,\"category\":\"none\",\"confidence\":0,\"reason_code\":\"\",\"action\":\"allow\"}"}}]}`))
	}))
	defer server.Close()
	result, err := EvaluateContentAudit(context.Background(), ContentAuditConfig{
		Enabled: true, Endpoint: server.URL + "/v1", APIKey: "test-key", Model: "audit-model", Timeout: time.Second,
	}, "hello")
	require.NoError(t, err)
	require.False(t, result.Flagged)
}

func TestContentAuditHealthReportsConfigurationWithoutSecrets(t *testing.T) {
	t.Setenv("CONTENT_AUDIT_ENABLED", "true")
	t.Setenv("CONTENT_AUDIT_ENDPOINT", "https://audit.example.invalid/v1/chat/completions")
	t.Setenv("CONTENT_AUDIT_API_KEY", "do-not-return-this")
	t.Setenv("CONTENT_AUDIT_MODEL", "safe-audit-model")
	t.Setenv("CONTENT_AUDIT_TIMEOUT_MS", "3500")
	t.Setenv("CONTENT_AUDIT_SAMPLE_RATE", "0.25")
	health := ContentAuditHealthSnapshot()
	require.True(t, health.Enabled)
	require.True(t, health.Configured)
	require.Equal(t, "safe-audit-model", health.Model)
	require.Equal(t, int64(3500), health.TimeoutMS)
	require.Equal(t, 0.25, health.SampleRate)
}

func TestEnqueueContentAuditShadowCreatesPendingReview(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AbuseReview{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"flagged\":true,\"category\":\"cyber_abuse\",\"confidence\":0.88,\"reason_code\":\"third_party_attack\",\"action\":\"review\"}"}}]}`))
	}))
	defer server.Close()
	t.Setenv("CONTENT_AUDIT_ENABLED", "true")
	t.Setenv("CONTENT_AUDIT_ENDPOINT", server.URL)
	t.Setenv("CONTENT_AUDIT_API_KEY", "test-key")
	t.Setenv("CONTENT_AUDIT_MODEL", "audit-model")
	t.Setenv("CONTENT_AUDIT_TIMEOUT_MS", "1000")
	t.Setenv("CONTENT_AUDIT_SAMPLE_RATE", "1")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("request_id", "content-audit-test")
	EnqueueContentAuditShadow(c, 17, 19, "gpt-test", "review this input")

	require.Eventually(t, func() bool {
		var count int64
		return db.Model(&model.AbuseReview{}).Where("action = ?", "abuse.content_audit").Count(&count).Error == nil && count == 1
	}, time.Second, 10*time.Millisecond)
	var review model.AbuseReview
	require.NoError(t, db.Where("action = ?", "abuse.content_audit").First(&review).Error)
	require.Equal(t, model.AbuseReviewStatusPending, review.Status)
	require.Equal(t, 17, review.UserId)
	require.Equal(t, 19, review.TokenId)
}
