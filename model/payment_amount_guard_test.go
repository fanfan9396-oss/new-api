package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRechargeEpayWithAmountRejectsCallbackAmountMismatch(t *testing.T) {
	truncateTables(t)

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 507, 0)
	order := createEpayTestOrder(t, user.Id, "EPAYTESTAMOUNT", PaymentProviderEpay, common.TopUpStatusPending)
	order.Amount = 10
	order.Money = 10.50
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", order.Id).Updates(map[string]any{"amount": order.Amount, "money": order.Money}).Error)

	_, err := RechargeEpayWithAmount(order.TradeNo, "wxpay", "10.49", "127.0.0.1")
	assert.ErrorIs(t, err, ErrTopUpAmountMismatch)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))

	alreadyDone, err := RechargeEpayWithAmount(order.TradeNo, "wxpay", "10.50", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 10*500000, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
}
