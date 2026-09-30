package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registrationPolicyTestContext(t *testing.T, payload string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewBufferString(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func configureRegistrationPolicyTest(t *testing.T) {
	t.Helper()
	previousRegister := common.RegisterEnabled
	previousPasswordRegister := common.PasswordRegisterEnabled
	previousEmailVerification := common.EmailVerificationEnabled
	previousQuota := common.QuotaForNewUser
	previousDefaultToken := constant.GenerateDefaultToken
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0
	constant.GenerateDefaultToken = false
	t.Cleanup(func() {
		common.RegisterEnabled = previousRegister
		common.PasswordRegisterEnabled = previousPasswordRegister
		common.EmailVerificationEnabled = previousEmailVerification
		common.QuotaForNewUser = previousQuota
		constant.GenerateDefaultToken = previousDefaultToken
	})
}

func TestRegisterCreatesCommonUserWithZeroWalletAndIgnoresAdminFields(t *testing.T) {
	dbUser, _ := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	configureRegistrationPolicyTest(t)
	require.NoError(t, i18n.Init())

	ctx, recorder := registrationPolicyTestContext(t, `{
		"username":"reg-policy-user",
		"password":"registration-password-123",
		"role":100,
		"status":2,
		"quota":999999,
		"used_quota":777,
		"group":"vip",
		"display_name":"Injected Root"
	}`)
	Register(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success, recorder.Body.String())

	var created model.User
	require.NoError(t, model.DB.Where("username = ?", "reg-policy-user").First(&created).Error)
	assert.Equal(t, common.RoleCommonUser, created.Role)
	assert.Equal(t, common.UserStatusEnabled, created.Status)
	assert.Equal(t, 0, created.Quota)
	assert.Equal(t, 0, created.UsedQuota)
	assert.Equal(t, "default", created.Group)
	assert.Equal(t, "reg-policy-user", created.DisplayName)
	assert.NotEqual(t, dbUser.Id, created.Id)

	var tokenCount int64
	require.NoError(t, model.DB.Model(&model.Token{}).Where("user_id = ?", created.Id).Count(&tokenCount).Error)
	assert.Zero(t, tokenCount)
}

func TestRegisterRejectsWhenRegistrationOrPasswordRegistrationDisabled(t *testing.T) {
	setupSecurityEnrollmentTest(t)
	configureRegistrationPolicyTest(t)
	require.NoError(t, i18n.Init())

	tests := []struct {
		name             string
		registerEnabled  bool
		passwordRegister bool
	}{
		{name: "registration disabled", registerEnabled: false, passwordRegister: true},
		{name: "password registration disabled", registerEnabled: true, passwordRegister: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			common.RegisterEnabled = test.registerEnabled
			common.PasswordRegisterEnabled = test.passwordRegister
			ctx, recorder := registrationPolicyTestContext(t, `{"username":"blocked-registration","password":"registration-password-123"}`)
			Register(ctx)
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success, recorder.Body.String())
		})
	}
}

func TestRegisterRequiresEmailVerificationWhenEnabled(t *testing.T) {
	setupSecurityEnrollmentTest(t)
	configureRegistrationPolicyTest(t)
	require.NoError(t, i18n.Init())
	common.EmailVerificationEnabled = true

	ctx, recorder := registrationPolicyTestContext(t, `{"username":"email-required-reg","password":"registration-password-123","email":"test@example.com"}`)
	Register(ctx)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success, recorder.Body.String())

	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "email-required-reg").Count(&count).Error)
	assert.Zero(t, count)
}

func configureAnonymousRouteRateLimitTest(t *testing.T) {
	t.Helper()
	previousRedis := common.RedisEnabled
	previousCriticalEnabled := common.CriticalRateLimitEnable
	previousCriticalNum := common.CriticalRateLimitNum
	previousCriticalDuration := common.CriticalRateLimitDuration
	previousTurnstile := common.TurnstileCheckEnabled
	common.RedisEnabled = false
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 2
	common.CriticalRateLimitDuration = 60
	common.TurnstileCheckEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedis
		common.CriticalRateLimitEnable = previousCriticalEnabled
		common.CriticalRateLimitNum = previousCriticalNum
		common.CriticalRateLimitDuration = previousCriticalDuration
		common.TurnstileCheckEnabled = previousTurnstile
	})
}

func anonymousRouteRequest(t *testing.T, router http.Handler, method, path, remoteAddr, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.RemoteAddr = remoteAddr
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAnonymousRegistrationRouteEnforcesCriticalRateLimit(t *testing.T) {
	setupSecurityEnrollmentTest(t)
	configureRegistrationPolicyTest(t)
	configureAnonymousRouteRateLimitTest(t)
	require.NoError(t, i18n.Init())

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.POST(
		"/api/user/register",
		middleware.CriticalRateLimit(),
		middleware.AnonymousRequestBodyLimit(),
		middleware.TurnstileCheck(),
		Register,
	)

	remoteAddr := "203.0.113.221:41001"
	for i := 0; i < 2; i++ {
		body := fmt.Sprintf(`{"username":"route-reg-%d","password":"registration-password-123"}`, i)
		response := anonymousRouteRequest(t, router, http.MethodPost, "/api/user/register", remoteAddr, body)
		assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var payload struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		assert.True(t, payload.Success, response.Body.String())
	}

	limited := anonymousRouteRequest(t, router, http.MethodPost, "/api/user/register", remoteAddr, `{"username":"route-reg-blocked","password":"registration-password-123"}`)
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
}

func TestAnonymousLoginRouteEnforcesCriticalRateLimit(t *testing.T) {
	setupSecurityEnrollmentTest(t)
	configureAnonymousRouteRateLimitTest(t)
	require.NoError(t, i18n.Init())
	previousPasswordLogin := common.PasswordLoginEnabled
	common.PasswordLoginEnabled = true
	t.Cleanup(func() { common.PasswordLoginEnabled = previousPasswordLogin })

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.POST(
		"/api/user/login",
		middleware.CriticalRateLimit(),
		middleware.DisableCache(),
		middleware.AnonymousRequestBodyLimit(),
		middleware.TurnstileCheck(),
		Login,
	)

	remoteAddr := "203.0.113.222:41002"
	for i := 0; i < 2; i++ {
		response := anonymousRouteRequest(t, router, http.MethodPost, "/api/user/login", remoteAddr, `{"username":"enrollment-user","password":"enrollment-password"}`)
		assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var payload struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		assert.True(t, payload.Success, response.Body.String())
	}

	limited := anonymousRouteRequest(t, router, http.MethodPost, "/api/user/login", remoteAddr, `{"username":"enrollment-user","password":"enrollment-password"}`)
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
}
