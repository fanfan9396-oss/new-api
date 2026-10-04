package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeginAndCompleteWalletRefundFreezesAndDeductsAtomically(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&WalletRefund{}))
	user := User{Username: "refund-state-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Quota: 100}
	require.NoError(t, DB.Create(&user).Error)

	refund, err := BeginWalletRefund(user.Id, common.RoleRootUser, 99, "trade-refund-1", "manual review")
	require.NoError(t, err)
	assert.Equal(t, WalletRefundStatusReviewing, refund.Status)
	var frozen User
	require.NoError(t, DB.First(&frozen, user.Id).Error)
	assert.True(t, frozen.WalletFrozen)

	completed, err := CompleteWalletRefund(refund.ID, 40, "paid offline", "proof-ref-1")
	require.NoError(t, err)
	assert.Equal(t, WalletRefundStatusCompleted, completed.Status)
	assert.Equal(t, 40, completed.DeductQuota)
	var updated User
	require.NoError(t, DB.First(&updated, user.Id).Error)
	assert.Equal(t, 60, updated.Quota)
	assert.False(t, updated.WalletFrozen)
	repeated, err := CompleteWalletRefund(refund.ID, 40, "duplicate", "duplicate-proof")
	require.NoError(t, err)
	assert.Equal(t, WalletRefundStatusCompleted, repeated.Status)
	require.NoError(t, DB.First(&updated, user.Id).Error)
	assert.Equal(t, 60, updated.Quota)
}

func TestCompleteWalletRefundRejectsNegativeBalanceAndIsIdempotent(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&WalletRefund{}))
	user := User{Username: "refund-negative-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Quota: 10}
	require.NoError(t, DB.Create(&user).Error)
	refund, err := BeginWalletRefund(user.Id, common.RoleRootUser, 99, "trade-refund-2", "manual review")
	require.NoError(t, err)

	_, err = CompleteWalletRefund(refund.ID, 11, "paid", "proof-ref-2")
	assert.ErrorIs(t, err, ErrWalletQuotaInsufficientForRefund)
	var stillFrozen User
	require.NoError(t, DB.First(&stillFrozen, user.Id).Error)
	assert.True(t, stillFrozen.WalletFrozen)

	require.NoError(t, CancelWalletRefund(refund.ID, "rejected"))
	require.NoError(t, CancelWalletRefund(refund.ID, "rejected again"))
	var unfrozen User
	require.NoError(t, DB.First(&unfrozen, user.Id).Error)
	assert.False(t, unfrozen.WalletFrozen)
}

func TestGetActiveWalletRefundFindsReviewingRecord(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&WalletRefund{}))
	user := User{Username: "refund-lookup-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Quota: 10}
	require.NoError(t, DB.Create(&user).Error)
	created, err := BeginWalletRefund(user.Id, common.RoleRootUser, 99, "trade-refund-lookup", "manual review")
	require.NoError(t, err)
	found, err := GetActiveWalletRefund(created.TradeNo)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
}

func TestCancelledWalletRefundCanBeRestarted(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&WalletRefund{}))
	user := User{Username: "refund-restart-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Quota: 10}
	require.NoError(t, DB.Create(&user).Error)
	created, err := BeginWalletRefund(user.Id, common.RoleRootUser, 99, "trade-refund-restart", "first review")
	require.NoError(t, err)
	require.NoError(t, CancelWalletRefund(created.ID, "cancelled first attempt"))

	restarted, err := BeginWalletRefund(user.Id, common.RoleRootUser, 100, "trade-refund-restart", "second review")
	require.NoError(t, err)
	assert.Equal(t, created.ID, restarted.ID)
	assert.Equal(t, WalletRefundStatusReviewing, restarted.Status)
	assert.Equal(t, "second review", restarted.Reason)
	var frozen User
	require.NoError(t, DB.First(&frozen, user.Id).Error)
	assert.True(t, frozen.WalletFrozen)
}
