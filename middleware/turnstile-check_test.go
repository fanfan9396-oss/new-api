package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTurnstileCheckRejectsMissingToken(t *testing.T) {
	previous := common.TurnstileCheckEnabled
	common.TurnstileCheckEnabled = true
	t.Cleanup(func() { common.TurnstileCheckEnabled = previous })

	router := gin.New()
	router.GET("/register", TurnstileCheck(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/register", nil)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "Turnstile token")
}

func TestTurnstileCheckUsesInjectedVerifierAndContinuesOnSuccess(t *testing.T) {
	previousEnabled := common.TurnstileCheckEnabled
	previousVerifier := verifyTurnstileToken
	common.TurnstileCheckEnabled = true
	verifyTurnstileToken = func(ctx context.Context, token, remoteIP string) (bool, error) {
		require.Equal(t, "test-token", token)
		require.NotEmpty(t, remoteIP)
		return true, nil
	}
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previousEnabled
		verifyTurnstileToken = previousVerifier
	})

	router := gin.New()
	router.GET("/register", TurnstileCheck(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/register?turnstile=test-token", nil)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestTurnstileCheckDoesNotExposeVerifierError(t *testing.T) {
	previousEnabled := common.TurnstileCheckEnabled
	previousVerifier := verifyTurnstileToken
	common.TurnstileCheckEnabled = true
	verifyTurnstileToken = func(context.Context, string, string) (bool, error) {
		return false, errors.New("secret upstream detail")
	}
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previousEnabled
		verifyTurnstileToken = previousVerifier
	})

	router := gin.New()
	router.GET("/register", TurnstileCheck(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/register?turnstile=test-token", nil)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.NotContains(t, response.Body.String(), "secret upstream detail")
	require.Contains(t, response.Body.String(), "人机验证服务暂不可用")
}

func TestTurnstileCheckRejectsFailedVerification(t *testing.T) {
	previousEnabled := common.TurnstileCheckEnabled
	previousVerifier := verifyTurnstileToken
	common.TurnstileCheckEnabled = true
	verifyTurnstileToken = func(context.Context, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previousEnabled
		verifyTurnstileToken = previousVerifier
	})

	router := gin.New()
	router.GET("/register", TurnstileCheck(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/register?turnstile=test-token", nil)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "Turnstile 校验失败")
}
