package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAbuseReviewModelTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AbuseReview{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })
	return db
}

func TestAbuseReviewScoreAndDisposition(t *testing.T) {
	db := setupAbuseReviewModelTest(t)
	now := common.GetTimestamp()
	require.NoError(t, CreateAbuseReview(&AbuseReview{
		EventId: "event-a", UserId: 7, TokenId: 8, Action: "abuse.provider_error",
		Status: AbuseReviewStatusPending, RiskScore: 2, CreatedAt: now,
	}))
	score := AbuseAggregateScore(7, 8, "retry_storm", AuditFields{"attempts": 3})
	assert.Equal(t, 4, score)
	assert.Equal(t, "observe", AbuseDisposition(2))
	assert.Equal(t, "review", AbuseDisposition(3))
	assert.Equal(t, "manual_action", AbuseDisposition(6))
	var count int64
	require.NoError(t, db.Model(&AbuseReview{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAbuseReviewCreateIsIdempotentAndExpiresFromAggregateWindow(t *testing.T) {
	db := setupAbuseReviewModelTest(t)
	now := common.GetTimestamp()
	review := &AbuseReview{EventId: "duplicate-event", UserId: 9, TokenId: 10, Action: "abuse.token_churn", Status: AbuseReviewStatusPending, RiskScore: 2, CreatedAt: now}
	require.NoError(t, CreateAbuseReview(review))
	require.NoError(t, CreateAbuseReview(&AbuseReview{EventId: review.EventId, UserId: 9, TokenId: 10, Action: review.Action, Status: AbuseReviewStatusPending, RiskScore: 2, CreatedAt: now}))
	var count int64
	require.NoError(t, db.Model(&AbuseReview{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, CreateAbuseReview(&AbuseReview{
		EventId: "old-event", UserId: 11, TokenId: 12, Action: "abuse.provider_error",
		Status: AbuseReviewStatusPending, RiskScore: 2,
		CreatedAt: now - int64((AbuseReviewWindow+time.Second)/time.Second),
	}))
	assert.Equal(t, 2, AbuseAggregateScore(11, 12, "retry_storm", AuditFields{"attempts": 3}))
	assert.Equal(t, 4, AbuseAggregateScore(9, 10, "retry_storm", AuditFields{"attempts": 3}))
}

func TestAbuseReviewRejectsInvalidInput(t *testing.T) {
	setupAbuseReviewModelTest(t)
	assert.Error(t, CreateAbuseReview(nil))
	assert.Error(t, CreateAbuseReview(&AbuseReview{EventId: "x", UserId: 1, TokenId: 1, Action: "abuse.x", Status: "closed"}))
	assert.Error(t, UpdateAbuseReview(&AbuseReview{Id: 1, Status: "closed"}))
	assert.False(t, ValidAbuseReviewTransition(AbuseReviewStatusPending))
	assert.True(t, ValidAbuseReviewTransition(AbuseReviewStatusResolved))
}
