package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	WalletRefundStatusReviewing = "reviewing"
	WalletRefundStatusCompleted = "completed"
	WalletRefundStatusCancelled = "cancelled"
)

var (
	ErrWalletRefundInvalid              = errors.New("invalid wallet refund")
	ErrWalletRefundAlreadyActive        = errors.New("wallet refund already active")
	ErrWalletRefundState                = errors.New("wallet refund state invalid")
	ErrWalletQuotaInsufficientForRefund = errors.New("wallet quota insufficient for refund")
)

// WalletRefund records an administrator's offline refund processing state.
// It deliberately has no payment-provider refund fields: the product policy
// uses an offline payout and a separate manual wallet deduction.
type WalletRefund struct {
	ID          int    `json:"id"`
	UserID      int    `json:"user_id" gorm:"index"`
	TradeNo     string `json:"trade_no" gorm:"uniqueIndex;type:varchar(255)"`
	Status      string `json:"status" gorm:"type:varchar(32);index"`
	Reason      string `json:"reason" gorm:"type:text"`
	ProofRef    string `json:"proof_ref" gorm:"type:varchar(512)"`
	DeductQuota int    `json:"deduct_quota" gorm:"default:0"`
	OperatorID  int    `json:"operator_id" gorm:"index"`
	CreatedAt   int64  `json:"created_at" gorm:"index"`
	UpdatedAt   int64  `json:"updated_at"`
	CompletedAt int64  `json:"completed_at"`
}

func setWalletFrozenTx(tx *gorm.DB, userID int, frozen bool) error {
	if _, err := IncrementUserAuthVersionWithTx(tx, userID); err != nil {
		return err
	}
	result := tx.Model(&User{}).Where("id = ?", userID).Update("wallet_frozen", frozen)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func BeginWalletRefund(userID, operatorRole, operatorID int, tradeNo, reason string) (*WalletRefund, error) {
	if userID <= 0 || operatorRole < common.RoleAdminUser || strings.TrimSpace(tradeNo) == "" || strings.TrimSpace(reason) == "" {
		return nil, ErrWalletRefundInvalid
	}
	refund := &WalletRefund{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		if operatorRole != common.RoleRootUser && operatorRole <= user.Role {
			return ErrUserQuotaPermission
		}
		var existing WalletRefund
		if err := tx.Where("trade_no = ?", tradeNo).First(&existing).Error; err == nil {
			return ErrWalletRefundAlreadyActive
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if user.WalletFrozen {
			return ErrWalletRefundAlreadyActive
		}
		if err := setWalletFrozenTx(tx, userID, true); err != nil {
			return err
		}
		now := common.GetTimestamp()
		refund = &WalletRefund{UserID: userID, TradeNo: tradeNo, Status: WalletRefundStatusReviewing, Reason: reason, OperatorID: operatorID, CreatedAt: now, UpdatedAt: now}
		return tx.Create(refund).Error
	})
	if err != nil {
		return nil, err
	}
	if err := PublishUserAuthCache(userID); err != nil {
		return nil, err
	}
	return refund, nil
}

func CompleteWalletRefund(refundID, deductQuota int, reason, proofRef string) (*WalletRefund, error) {
	if refundID <= 0 || deductQuota <= 0 || strings.TrimSpace(reason) == "" || strings.TrimSpace(proofRef) == "" {
		return nil, ErrWalletRefundInvalid
	}
	refund := &WalletRefund{}
	userID := 0
	justCompleted := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(refund, refundID).Error; err != nil {
			return err
		}
		if refund.Status == WalletRefundStatusCompleted {
			userID = refund.UserID
			return nil
		}
		if refund.Status != WalletRefundStatusReviewing {
			return ErrWalletRefundState
		}
		var user User
		if err := lockForUpdate(tx).First(&user, refund.UserID).Error; err != nil {
			return err
		}
		if !user.WalletFrozen {
			return ErrWalletRefundState
		}
		result := tx.Model(&User{}).Where("id = ? AND quota >= ?", user.Id, deductQuota).Update("quota", gorm.Expr("quota - ?", deductQuota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrWalletQuotaInsufficientForRefund
		}
		now := common.GetTimestamp()
		refund.Status = WalletRefundStatusCompleted
		refund.DeductQuota = deductQuota
		refund.Reason = reason
		refund.ProofRef = proofRef
		refund.UpdatedAt = now
		refund.CompletedAt = now
		if err := tx.Save(refund).Error; err != nil {
			return err
		}
		if err := setWalletFrozenTx(tx, user.Id, false); err != nil {
			return err
		}
		userID = user.Id
		justCompleted = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if justCompleted && refund.DeductQuota > 0 {
		if err := cacheIncrUserQuota(userID, int64(-refund.DeductQuota)); err != nil {
			common.SysError("failed to sync wallet refund quota cache: " + err.Error())
		}
	}
	if err := PublishUserAuthCache(userID); err != nil {
		return nil, err
	}
	return refund, nil
}

func CancelWalletRefund(refundID int, reason string) error {
	if refundID <= 0 || strings.TrimSpace(reason) == "" {
		return ErrWalletRefundInvalid
	}
	var userID int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var refund WalletRefund
		if err := lockForUpdate(tx).First(&refund, refundID).Error; err != nil {
			return err
		}
		if refund.Status == WalletRefundStatusCancelled {
			userID = refund.UserID
			return nil
		}
		if refund.Status != WalletRefundStatusReviewing {
			return ErrWalletRefundState
		}
		if err := setWalletFrozenTx(tx, refund.UserID, false); err != nil {
			return err
		}
		refund.Status = WalletRefundStatusCancelled
		refund.Reason = reason
		refund.UpdatedAt = common.GetTimestamp()
		userID = refund.UserID
		return tx.Save(&refund).Error
	})
	if err != nil {
		return err
	}
	return PublishUserAuthCache(userID)
}
