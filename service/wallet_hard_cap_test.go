package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// These tests encode the confirmed product contract:
// an Unlimited token may skip its own token cap, but it must not bypass the
// authenticated user's wallet hard cap or create a negative wallet balance.
func TestWalletHardCapTrustedUnlimitedDoesNotBypassWallet(t *testing.T) {
	truncate(t)

	oldTrust := operation_setting.GetQuotaSetting().TrustQuotaUSD
	oldQuotaPerUnit := common.QuotaPerUnit
	operation_setting.GetQuotaSetting().TrustQuotaUSD = 1
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		operation_setting.GetQuotaSetting().TrustQuotaUSD = oldTrust
		common.QuotaPerUnit = oldQuotaPerUnit
	})

	const userID = 8101
	trustThreshold := int(common.QuotaPerUnit)
	wallet := trustThreshold + 100
	requested := trustThreshold + 200
	seedUser(t, userID, wallet)

	relayInfo := &relaycommon.RelayInfo{
		UserId:         userID,
		UserQuota:      wallet,
		TokenUnlimited: true,
	}
	session := &BillingSession{
		relayInfo: relayInfo,
		funding:   &WalletFunding{userId: userID},
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	err := session.preConsume(ctx, requested)
	if err == nil {
		t.Fatalf("preConsume admitted a request above wallet cap: preConsumed=%d wallet=%d", session.preConsumedQuota, getUserQuota(t, userID))
	}
	// A request whose estimated charge already exceeds the user's wallet must
	// not be admitted by the trust-quota fast path.
	assert.Equal(t, wallet, getUserQuota(t, userID))
}

func TestWalletHardCapSettlementRejectsWalletDebt(t *testing.T) {
	truncate(t)

	const userID = 8102
	const wallet = 100
	const actual = 200
	seedUser(t, userID, wallet)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       userID,
		IsPlayground: true,
	}
	session := &BillingSession{
		relayInfo:        relayInfo,
		funding:          &WalletFunding{userId: userID},
		preConsumedQuota: 0,
	}

	err := session.Settle(actual)
	// A final usage amount above the wallet must not be settled into a
	// negative balance. The request should be rejected or otherwise returned
	// as an explicit insufficient-wallet result by the owner boundary.
	if err == nil {
		t.Fatalf("settlement admitted wallet debt: wallet=%d", getUserQuota(t, userID))
	}
	assert.Equal(t, wallet, getUserQuota(t, userID))
}

func TestWalletHardCapLegacyPostConsumeRejectsWalletDebt(t *testing.T) {
	truncate(t)

	const userID = 8104
	const wallet = 100
	const actual = 200
	seedUser(t, userID, wallet)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       userID,
		IsPlayground: true,
	}

	err := PostConsumeQuota(relayInfo, actual, 0, false)
	if err == nil {
		t.Fatalf("legacy post-consume admitted wallet debt: wallet=%d", getUserQuota(t, userID))
	}
	assert.Equal(t, wallet, getUserQuota(t, userID))
}

func TestWalletHardCapTaskSettlementRejectsWalletDebt(t *testing.T) {
	truncate(t)

	const userID = 8105
	const wallet = 100
	seedUser(t, userID, wallet)

	err := taskAdjustFunding(&model.Task{UserId: userID}, 200)
	if err == nil {
		t.Fatalf("task settlement admitted wallet debt: wallet=%d", getUserQuota(t, userID))
	}
	assert.Equal(t, wallet, getUserQuota(t, userID))
}

func TestWalletHardCapMultipleUnlimitedKeysShareUserWallet(t *testing.T) {
	truncate(t)

	const userID = 8103
	const wallet = 100
	seedUser(t, userID, wallet)

	ctx1, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx2, _ := gin.CreateTestContext(httptest.NewRecorder())
	first := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{UserId: userID, UserQuota: wallet, TokenUnlimited: true, IsPlayground: true},
		funding:   &WalletFunding{userId: userID},
	}
	second := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{UserId: userID, UserQuota: wallet, TokenUnlimited: true, IsPlayground: true},
		funding:   &WalletFunding{userId: userID},
	}

	if err := first.preConsume(ctx1, 60); err != nil {
		t.Fatalf("first key was rejected unexpectedly: %v", err)
	}
	if err := second.preConsume(ctx2, 60); err == nil {
		t.Fatalf("second unlimited key bypassed shared user wallet: wallet=%d", getUserQuota(t, userID))
	}
	assert.Equal(t, 40, getUserQuota(t, userID))
}
