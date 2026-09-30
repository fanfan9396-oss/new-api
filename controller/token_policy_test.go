package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenPolicyFieldsAreOwnedByTheAuthenticatedUser verifies that a member
// may configure only the intended token-level restrictions. Client-supplied
// ownership/accounting fields must not move the token to another user or alter
// the user's wallet.
func TestTokenPolicyFieldsAreOwnedByTheAuthenticatedUser(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	owner := &model.User{
		Id:       101,
		Username: "token-policy-owner",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Quota:    12345,
		AffCode:  "token-policy-owner-aff",
	}
	other := &model.User{
		Id:       202,
		Username: "token-policy-other",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Quota:    67890,
		AffCode:  "token-policy-other-aff",
	}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(other).Error)

	allowIPs := "203.0.113.10\n203.0.113.11"
	request := map[string]any{
		"user_id":              other.Id,
		"name":                 "member-policy-key",
		"expired_time":         int64(-1),
		"remain_quota":         250,
		"unlimited_quota":      true,
		"model_limits_enabled": true,
		"model_limits":         "mock-chat,mock-error-429",
		"allow_ips":            allowIPs,
		"cross_group_retry":    true,
		"status":               common.TokenStatusDisabled,
		"used_quota":           999999,
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", request, owner.Id)
	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var token model.Token
	require.NoError(t, db.Where("name = ?", "member-policy-key").First(&token).Error)
	assert.Equal(t, owner.Id, token.UserId, "token ownership must come from authenticated user")
	assert.NotEqual(t, other.Id, token.UserId)
	assert.Equal(t, common.TokenStatusEnabled, token.Status, "client status must not bypass creation defaults")
	assert.Equal(t, 250, token.RemainQuota)
	assert.True(t, token.UnlimitedQuota)
	assert.True(t, token.ModelLimitsEnabled)
	assert.Equal(t, map[string]bool{"mock-chat": true, "mock-error-429": true}, token.GetModelLimitsMap())
	require.NotNil(t, token.AllowIps)
	assert.Equal(t, allowIPs, *token.AllowIps)
	assert.False(t, token.CrossGroupRetry, "cross-group retry is only valid for auto group")
	assert.Zero(t, token.UsedQuota, "client accounting fields must not seed usage")

	var refreshedOwner, refreshedOther model.User
	require.NoError(t, db.First(&refreshedOwner, owner.Id).Error)
	require.NoError(t, db.First(&refreshedOther, other.Id).Error)
	assert.Equal(t, owner.Quota, refreshedOwner.Quota, "creating a key must not change the wallet")
	assert.Equal(t, other.Quota, refreshedOther.Quota)
}

// TestUpdateTokenPolicyFieldsCannotTransferOwnership verifies that update
// payloads can change token restrictions but cannot transfer ownership or
// write account-level fields through the embedded Token JSON shape.
func TestUpdateTokenPolicyFieldsCannotTransferOwnership(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	owner := &model.User{
		Id:       303,
		Username: "token-update-owner",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Quota:    4321,
		AffCode:  "token-update-owner-aff",
	}
	other := &model.User{
		Id:       404,
		Username: "token-update-other",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Quota:    8765,
		AffCode:  "token-update-other-aff",
	}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(other).Error)

	token := &model.Token{
		UserId:         owner.Id,
		Name:           "owned-policy-key",
		Key:            "owned-policy-key-secret",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    100,
		UnlimitedQuota: true,
		Group:          "default",
	}
	require.NoError(t, db.Create(token).Error)

	request := map[string]any{
		"id":                   token.Id,
		"user_id":              other.Id,
		"name":                 "owned-policy-key-updated",
		"expired_time":         int64(-1),
		"remain_quota":         25,
		"unlimited_quota":      false,
		"model_limits_enabled": true,
		"model_limits":         "mock-chat",
		"allow_ips":            "198.51.100.20/32",
		"status":               common.TokenStatusDisabled,
		"used_quota":           999999,
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", request, owner.Id)
	UpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var updated model.Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, owner.Id, updated.UserId)
	assert.Equal(t, common.TokenStatusEnabled, updated.Status, "normal update must not accept client status changes")
	assert.Equal(t, 25, updated.RemainQuota)
	assert.False(t, updated.UnlimitedQuota)
	assert.True(t, updated.ModelLimitsEnabled)
	assert.Equal(t, map[string]bool{"mock-chat": true}, updated.GetModelLimitsMap())
	require.NotNil(t, updated.AllowIps)
	assert.Equal(t, "198.51.100.20/32", *updated.AllowIps)
	assert.Zero(t, updated.UsedQuota)

	var refreshedOwner, refreshedOther model.User
	require.NoError(t, db.First(&refreshedOwner, owner.Id).Error)
	require.NoError(t, db.First(&refreshedOther, other.Id).Error)
	assert.Equal(t, owner.Quota, refreshedOwner.Quota)
	assert.Equal(t, other.Quota, refreshedOther.Quota)
}
