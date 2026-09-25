package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEpayNotifyValidatesSignatureAmountAndIdempotency(t *testing.T) {
	db := openTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))

	oldAddress := operation_setting.PayAddress
	oldID := operation_setting.EpayId
	oldKey := operation_setting.EpayKey
	oldMethods := operation_setting.PayMethods
	paymentSetting := operation_setting.GetPaymentSetting()
	oldConfirmed := paymentSetting.ComplianceConfirmed
	oldTerms := paymentSetting.ComplianceTermsVersion
	oldQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() {
		operation_setting.PayAddress = oldAddress
		operation_setting.EpayId = oldID
		operation_setting.EpayKey = oldKey
		operation_setting.PayMethods = oldMethods
		paymentSetting.ComplianceConfirmed = oldConfirmed
		paymentSetting.ComplianceTermsVersion = oldTerms
		common.QuotaPerUnit = oldQuotaPerUnit
	})

	operation_setting.PayAddress = "https://pay.test"
	operation_setting.EpayId = "test-pid"
	operation_setting.EpayKey = "test-key"
	operation_setting.PayMethods = []map[string]string{{"name": "微信", "type": "wxpay"}}
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	common.QuotaPerUnit = 500000

	user := &model.User{Id: 508, Username: "epay-webhook-user", Password: "password", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	order := &model.TopUp{UserId: user.Id, Amount: 10, Money: 10.50, TradeNo: "EPAYCONTROLLERAMOUNT", PaymentMethod: "wxpay", PaymentProvider: model.PaymentProviderEpay, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
	require.NoError(t, model.DB.Create(order).Error)

	makeRequest := func(money string, tamperAfterSign bool) *httptest.ResponseRecorder {
		params := map[string]string{
			"pid":          operation_setting.EpayId,
			"trade_no":     "gateway-trade-1",
			"out_trade_no": order.TradeNo,
			"type":         "wxpay",
			"name":         "Wallet 10",
			"money":        money,
			"trade_status": epay.StatusTradeSuccess,
		}
		params = epay.GenerateParams(params, operation_setting.EpayKey)
		if tamperAfterSign {
			params["money"] = "10.51"
		}
		form := url.Values{}
		for key, value := range params {
			form.Set(key, value)
		}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/epay/notify", strings.NewReader(form.Encode()))
		ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		EpayNotify(ctx)
		return recorder
	}

	badSignature := makeRequest("10.50", true)
	assert.Equal(t, "fail", badSignature.Body.String())

	bad := makeRequest("10.49", false)
	assert.Equal(t, "fail", bad.Body.String())
	var stored model.TopUp
	require.NoError(t, model.DB.First(&stored, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
	var storedUser model.User
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, 0, storedUser.Quota)

	good := makeRequest("10.50", false)
	assert.Equal(t, "success", good.Body.String())
	require.NoError(t, model.DB.First(&stored, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, 10*500000, storedUser.Quota)

	duplicate := makeRequest("10.50", false)
	assert.Equal(t, "success", duplicate.Body.String())
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, 10*500000, storedUser.Quota)
}
