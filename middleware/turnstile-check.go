package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

type turnstileCheckResponse struct {
	Success bool `json:"success"`
}

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// verifyTurnstileToken is kept injectable so registration/login tests never call
// the external Turnstile service. The production implementation has a bounded
// timeout and never returns provider error text to the client.
var verifyTurnstileToken = func(ctx context.Context, token, remoteIP string) (bool, error) {
	form := url.Values{
		"secret":   {common.TurnstileSecretKey},
		"response": {token},
		"remoteip": {remoteIP},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf("turnstile verification returned HTTP %d", response.StatusCode)
	}

	var result turnstileCheckResponse
	if err := common.DecodeJson(response.Body, &result); err != nil {
		return false, err
	}
	return result.Success, nil
}

func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.TurnstileCheckEnabled {
			c.Next()
			return
		}

		token := c.Query("turnstile")
		if token == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Turnstile token 为空",
			})
			c.Abort()
			return
		}

		verified, err := verifyTurnstileToken(c.Request.Context(), token, c.ClientIP())
		if err != nil {
			logger.LogWarn(c.Request.Context(), "turnstile verification unavailable: %v", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "人机验证服务暂不可用，请稍后重试",
			})
			c.Abort()
			return
		}
		if !verified {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Turnstile 校验失败，请刷新重试！",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
