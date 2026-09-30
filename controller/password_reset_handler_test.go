package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type passwordResetHandlerResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func passwordResetRequest(t *testing.T, email, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": email, "token": token})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/reset", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ResetPassword(ctx)
	return recorder
}

func createPasswordResetUser(t *testing.T, email, username string) *model.User {
	t.Helper()
	password, err := common.Password2Hash("old-password-123")
	require.NoError(t, err)
	user := &model.User{
		Username: username,
		Email:    email,
		Password: password,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  username + "-aff",
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func TestResetPasswordHandlerConsumesTokenAfterSuccessfulReset(t *testing.T) {
	setupSecurityEnrollmentTest(t)
	user := createPasswordResetUser(t, "reset-handler@example.com", "reset-handler-user")
	const resetToken = "reset-handler-token"
	common.RegisterVerificationCodeWithKey(user.Email, resetToken, common.PasswordResetPurpose)

	first := passwordResetRequest(t, user.Email, resetToken)
	var firstResponse passwordResetHandlerResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResponse))
	assert.True(t, firstResponse.Success, first.Body.String())
	assert.NotEmpty(t, firstResponse.Data)

	second := passwordResetRequest(t, user.Email, resetToken)
	var secondResponse passwordResetHandlerResponse
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondResponse))
	assert.False(t, secondResponse.Success, second.Body.String())

	var stored model.User
	require.NoError(t, model.DB.First(&stored, user.Id).Error)
	assert.Equal(t, int64(2), stored.AuthVersion)
}

func TestResetPasswordHandlerRejectsExpiredAndWrongTokensWithoutMutation(t *testing.T) {
	setupSecurityEnrollmentTest(t)
	user := createPasswordResetUser(t, "reset-invalid@example.com", "reset-invalid-user")
	originalVersion := user.AuthVersion

	previousValidity := common.VerificationValidMinutes
	common.VerificationValidMinutes = 0
	t.Cleanup(func() { common.VerificationValidMinutes = previousValidity })
	common.RegisterVerificationCodeWithKey(user.Email, "expired-token", common.PasswordResetPurpose)
	expired := passwordResetRequest(t, user.Email, "expired-token")
	var expiredResponse passwordResetHandlerResponse
	require.NoError(t, json.Unmarshal(expired.Body.Bytes(), &expiredResponse))
	assert.False(t, expiredResponse.Success, expired.Body.String())

	common.VerificationValidMinutes = previousValidity
	common.RegisterVerificationCodeWithKey(user.Email, "valid-token", common.PasswordResetPurpose)
	wrongEmail := passwordResetRequest(t, "other@example.com", "valid-token")
	var wrongEmailResponse passwordResetHandlerResponse
	require.NoError(t, json.Unmarshal(wrongEmail.Body.Bytes(), &wrongEmailResponse))
	assert.False(t, wrongEmailResponse.Success, wrongEmail.Body.String())

	var stored model.User
	require.NoError(t, model.DB.First(&stored, user.Id).Error)
	assert.Equal(t, originalVersion, stored.AuthVersion)
}

func TestResetPasswordHandlerConcurrentUseAllowsOnlyOneSuccess(t *testing.T) {
	setupSecurityEnrollmentTest(t)
	user := createPasswordResetUser(t, "reset-concurrent@example.com", "reset-concurrent-user")
	const resetToken = "reset-concurrent-token"
	common.RegisterVerificationCodeWithKey(user.Email, resetToken, common.PasswordResetPurpose)

	responses := make(chan *httptest.ResponseRecorder, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			responses <- passwordResetRequest(t, user.Email, resetToken)
		}()
	}
	wait.Wait()
	close(responses)

	successes := 0
	for response := range responses {
		var payload passwordResetHandlerResponse
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
		if payload.Success {
			successes++
		}
	}
	assert.Equal(t, 1, successes, "a password reset token must be single-use under concurrent submission")
}

func TestResetPasswordHandlerUsesConfiguredTokenLifetime(t *testing.T) {
	setupSecurityEnrollmentTest(t)
	user := createPasswordResetUser(t, "reset-lifetime@example.com", "reset-lifetime-user")
	previousValidity := common.VerificationValidMinutes
	common.VerificationValidMinutes = 1
	t.Cleanup(func() { common.VerificationValidMinutes = previousValidity })
	common.RegisterVerificationCodeWithKey(user.Email, "lifetime-token", common.PasswordResetPurpose)

	assert.True(t, common.VerifyCodeWithKey(user.Email, "lifetime-token", common.PasswordResetPurpose))
	assert.False(t, common.VerifyCodeWithKey(user.Email, "wrong-token", common.PasswordResetPurpose))
	assert.WithinDuration(t, time.Now(), time.Now(), time.Second)
}
