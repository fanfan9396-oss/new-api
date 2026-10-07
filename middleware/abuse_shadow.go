package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const (
	abuseShadowWindow       = 10 * time.Minute
	abuseSignalCooldown     = time.Minute
	abuseLongRequestLimit   = 30 * time.Second
	abuseFailureSignalLimit = 3
)

// AbuseShadowMiddleware records behavior signals after relay requests finish.
// It is intentionally shadow-only: signals are audit records and do not block
// requests or mutate token, user, wallet, or provider state.
func AbuseShadowMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		if !common.RedisEnabled || common.RDB == nil {
			return
		}
		userID := c.GetInt(string(constant.ContextKeyUserId))
		tokenID := c.GetInt(string(constant.ContextKeyTokenId))
		if userID <= 0 || tokenID <= 0 {
			return
		}
		ctx := context.Background()
		prefix := fmt.Sprintf("abuse:v1:%d", userID)
		modelName := c.GetString(string(constant.ContextKeyOriginalModel))
		if modelName == "" {
			modelName = c.Request.URL.Path
		}
		ipRef := model.AccessTokenFingerprint(c.ClientIP())
		if len(ipRef) > 16 {
			ipRef = ipRef[:16]
		}
		if ipRef == "" {
			ipRef = "unknown"
		}
		observations := []struct {
			key   string
			value string
			kind  string
		}{
			{key: prefix + ":tokens", value: strconv.Itoa(tokenID), kind: "multi_token"},
			{key: prefix + ":ips", value: ipRef, kind: "multi_ip"},
			{key: prefix + ":models", value: modelName, kind: "model_switch"},
		}
		for _, observation := range observations {
			if err := common.RDB.SAdd(ctx, observation.key, observation.value).Err(); err != nil {
				continue
			}
			_ = common.RDB.Expire(ctx, observation.key, abuseShadowWindow).Err()
			count, err := common.RDB.SCard(ctx, observation.key).Result()
			if err != nil {
				continue
			}
			threshold := int64(2)
			if observation.kind == "multi_token" || observation.kind == "model_switch" {
				threshold = 3
			}
			if count >= threshold {
				emitAbuseSignal(c, prefix, observation.kind, map[string]any{
					"window_seconds": int(abuseShadowWindow / time.Second),
					"distinct_count": count,
					"model":          modelName,
				})
			}
		}

		status := c.Writer.Status()
		if attempts := service.RequestPolicy(c).Attempts; attempts >= 3 {
			emitAbuseSignal(c, prefix, "retry_storm", map[string]any{
				"attempts": attempts,
				"status":   status,
			})
		}
		if status >= 400 {
			key := prefix + ":failures"
			count, err := common.RDB.Incr(ctx, key).Result()
			if err == nil {
				_ = common.RDB.Expire(ctx, key, abuseShadowWindow).Err()
				if count >= abuseFailureSignalLimit {
					emitAbuseSignal(c, prefix, "failure_burst", map[string]any{
						"window_seconds": int(abuseShadowWindow / time.Second),
						"failure_count":  count,
						"status":         status,
					})
				}
			}
			if c.GetInt(string(constant.ContextKeyChannelId)) > 0 && (status == 401 || status == 403 || status == 429 || status >= 500) {
				emitAbuseSignal(c, prefix, "provider_error", map[string]any{
					"channel_id": c.GetInt(string(constant.ContextKeyChannelId)),
					"status":     status,
				})
			}
		}
		if time.Since(started) >= abuseLongRequestLimit {
			kind := "long_request"
			if common.GetContextKeyBool(c, constant.ContextKeyIsStream) {
				kind = "long_stream"
			}
			emitAbuseSignal(c, prefix, kind, map[string]any{
				"duration_ms": time.Since(started).Milliseconds(),
				"status":      status,
			})
		}
	}
}

// RecordTokenChurnSignal aggregates successful token lifecycle operations in
// a short window. It is a shadow signal only and does not reject the operation.
func RecordTokenChurnSignal(c *gin.Context, action string) {
	if c == nil || !common.RedisEnabled || common.RDB == nil || c.GetInt("id") <= 0 {
		return
	}
	ctx := context.Background()
	prefix := fmt.Sprintf("abuse:v1:%d", c.GetInt("id"))
	key := prefix + ":token-churn"
	count, err := common.RDB.Incr(ctx, key).Result()
	if err != nil {
		return
	}
	_ = common.RDB.Expire(ctx, key, abuseShadowWindow).Err()
	if count >= 3 {
		emitAbuseSignal(c, prefix, "token_churn", map[string]any{
			"window_seconds":  int(abuseShadowWindow / time.Second),
			"operation":       action,
			"operation_count": count,
		})
	}
}

func emitAbuseSignal(c *gin.Context, prefix, kind string, fields model.AuditFields) {
	if common.RDB == nil {
		return
	}
	cooldownKey := prefix + ":signal:" + kind
	set, err := common.RDB.SetNX(context.Background(), cooldownKey, "1", abuseSignalCooldown).Result()
	if err != nil || !set {
		return
	}
	model.RecordAbuseSignal(c, c.GetInt(string(constant.ContextKeyUserId)), c.GetInt(string(constant.ContextKeyTokenId)), kind, fields)
}
