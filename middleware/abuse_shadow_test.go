package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAbuseShadowRecordsDistinctTokenSignalWithoutRawIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AuditLog{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLogDB })
	_, _ = useRateLimitMiniRedis(t)

	router := gin.New()
	router.Use(AbuseShadowMiddleware())
	router.GET("/v1/chat/completions", func(c *gin.Context) {
		c.Set("id", 42)
		tokenID, _ := strconv.Atoi(c.Query("token"))
		c.Set("token_id", tokenID)
		c.Set("token_key", "member-secret-token")
		c.Set("original_model", "mock-chat")
		c.Status(http.StatusOK)
	})
	for tokenID := 1; tokenID <= 3; tokenID++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		rec := httptest.NewRecorder()
		// The handler reads this value without exposing it in the audit record.
		req.URL.RawQuery = "token=" + string(rune('0'+tokenID))
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	var signals []model.AuditLog
	require.NoError(t, db.Where("action = ?", "abuse.multi_token").Find(&signals).Error)
	require.Len(t, signals, 1)
	assert.Len(t, signals[0].Ip, 16)
	assert.NotEqual(t, "192.0.2.10", signals[0].Ip)
	assert.NotEmpty(t, signals[0].TokenRef)
}

func TestRecordTokenChurnSignalUsesShadowAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AuditLog{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLogDB })
	_, _ = useRateLimitMiniRedis(t)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 42)
	ctx.Set("token_key", "member-secret-token")
	for range 3 {
		RecordTokenChurnSignal(ctx, "token.create")
	}

	var signals []model.AuditLog
	require.NoError(t, db.Where("action = ?", "abuse.token_churn").Find(&signals).Error)
	require.Len(t, signals, 1)
	assert.Equal(t, 42, signals[0].UserId)
}

func TestAbuseShadowRecordsLongStreamRetryAndProviderFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AuditLog{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLogDB })
	_, _ = useRateLimitMiniRedis(t)
	previousLimit := abuseLongRequestLimit
	abuseLongRequestLimit = time.Millisecond
	t.Cleanup(func() { abuseLongRequestLimit = previousLimit })

	router := gin.New()
	router.Use(AbuseShadowMiddleware())
	router.GET("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, 42)
		common.SetContextKey(c, constant.ContextKeyTokenId, 7)
		common.SetContextKey(c, constant.ContextKeyTokenKey, "member-secret-token")
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "provider-failure-model")
		common.SetContextKey(c, constant.ContextKeyChannelId, 9)
		if c.Query("stream") == "true" {
			common.SetContextKey(c, constant.ContextKeyIsStream, true)
			time.Sleep(3 * time.Millisecond)
		}
		if c.Query("attempts") == "3" {
			for range 3 {
				service.RequestPolicy(c).BeginAttempt(&model.Channel{Id: 9}, "default")
			}
		}
		status := http.StatusBadGateway
		if c.Query("status") == "429" {
			status = http.StatusTooManyRequests
		}
		c.Status(status)
	})

	for _, query := range []string{
		"status=502&stream=true&attempts=3",
		"status=429",
		"status=502",
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions?"+query, nil)
		req.RemoteAddr = "192.0.2.10:1234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Contains(t, []int{http.StatusBadGateway, http.StatusTooManyRequests}, rec.Code)
	}

	var signals []model.AuditLog
	require.NoError(t, db.Where("action IN ?", []string{
		"abuse.long_stream", "abuse.retry_storm", "abuse.provider_error", "abuse.failure_burst",
	}).Find(&signals).Error)
	actions := map[string]bool{}
	for _, signal := range signals {
		actions[signal.Action] = true
	}
	for _, action := range []string{"abuse.long_stream", "abuse.retry_storm", "abuse.provider_error", "abuse.failure_burst"} {
		assert.True(t, actions[action], fmt.Sprintf("missing %s signal", action))
	}
}
