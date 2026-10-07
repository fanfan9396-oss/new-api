package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/model"
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
