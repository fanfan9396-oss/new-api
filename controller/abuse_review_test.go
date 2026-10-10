package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAbuseReviewQueueRequiresAuditPermissionAndClosesOnce(t *testing.T) {
	admin, pat := setupAccessTokenAudit(t)
	require.NoError(t, model.DB.AutoMigrate(&model.AbuseReview{}))
	review := &model.AbuseReview{
		EventId: "review-event-1", UserId: admin.Id, TokenId: 7,
		Action: "abuse.provider_error", Status: model.AbuseReviewStatusPending,
		RiskScore: 4, EvidenceJSON: `{"status":502}`,
	}
	require.NoError(t, model.CreateAbuseReview(review))

	router := gin.New()
	router.Use(middleware.RequestId(), middleware.AccessTokenAudit())
	router.GET("/api/abuse_reviews/summary", middleware.AdminAuth(), middleware.RequirePermission(authz.AuditRead), GetAbuseReviewSummary)
	router.GET("/api/abuse_reviews", middleware.AdminAuth(), middleware.RequirePermission(authz.AuditRead), GetAbuseReviews)
	router.PATCH("/api/abuse_reviews/:id", middleware.AdminAuth(), middleware.RequirePermission(authz.AuditRead), UpdateAbuseReview)

	denied := abuseReviewRequest(router, http.MethodGet, "/api/abuse_reviews", pat, "")
	assert.Equal(t, http.StatusForbidden, denied.Code)
	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{authz.ResourceAudit: {authz.ActionRead: true}}))

	listed := abuseReviewRequest(router, http.MethodGet, "/api/abuse_reviews?status=pending&action=abuse.provider_error", pat, "")
	assert.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.Contains(t, listed.Body.String(), `"event_id":"review-event-1"`)
	assert.Contains(t, listed.Body.String(), `"disposition":"review"`)
	summary := abuseReviewRequest(router, http.MethodGet, "/api/abuse_reviews/summary", pat, "")
	assert.Equal(t, http.StatusOK, summary.Code, summary.Body.String())
	assert.Contains(t, summary.Body.String(), `"content_audit":0`)
	assert.Contains(t, summary.Body.String(), `"enabled":false`)

	updated := abuseReviewRequest(router, http.MethodPatch, "/api/abuse_reviews/"+strconv.Itoa(review.Id), pat, `{"status":"resolved","note":"checked provider failure"}`)
	assert.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Contains(t, updated.Body.String(), `"status":"resolved"`)

	closed := abuseReviewRequest(router, http.MethodPatch, "/api/abuse_reviews/"+strconv.Itoa(review.Id), pat, `{"status":"false_positive"}`)
	assert.Equal(t, http.StatusOK, closed.Code)
	assert.Contains(t, closed.Body.String(), `"success":false`)

	invalid := abuseReviewRequest(router, http.MethodPatch, "/api/abuse_reviews/"+strconv.Itoa(review.Id), pat, `{"status":"pending"}`)
	assert.Equal(t, http.StatusOK, invalid.Code)
	assert.Contains(t, invalid.Body.String(), `"success":false`)
}

func TestAbuseReviewQueueRejectsInvalidFilters(t *testing.T) {
	admin, pat := setupAccessTokenAudit(t)
	require.NoError(t, model.DB.AutoMigrate(&model.AbuseReview{}))
	require.NoError(t, authz.SetUserPermissions(admin.Id, authz.PermissionsMap{authz.ResourceAudit: {authz.ActionRead: true}}))
	router := gin.New()
	router.GET("/api/abuse_reviews", middleware.AdminAuth(), middleware.RequirePermission(authz.AuditRead), GetAbuseReviews)
	response := abuseReviewRequest(router, http.MethodGet, "/api/abuse_reviews?action="+strings.Repeat("x", 129), pat, "")
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"success":false`)
}

func abuseReviewRequest(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.40:4567"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
