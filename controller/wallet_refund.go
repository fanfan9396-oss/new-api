package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type StartWalletRefundRequest struct {
	UserID  int    `json:"user_id" binding:"required"`
	TradeNo string `json:"trade_no" binding:"required"`
	Reason  string `json:"reason" binding:"required"`
}

type CompleteWalletRefundRequest struct {
	RefundID    int    `json:"refund_id" binding:"required"`
	DeductQuota int    `json:"deduct_quota" binding:"required"`
	Reason      string `json:"reason" binding:"required"`
	ProofRef    string `json:"proof_ref" binding:"required"`
}

type CancelWalletRefundRequest struct {
	RefundID int    `json:"refund_id" binding:"required"`
	Reason   string `json:"reason" binding:"required"`
}

func StartWalletRefund(c *gin.Context) {
	var req StartWalletRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, errors.New("invalid wallet refund parameters"))
		return
	}
	refund, err := model.BeginWalletRefund(req.UserID, c.GetInt("role"), c.GetInt("id"), req.TradeNo, req.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, req.UserID, "user.wallet_refund_start", map[string]any{
		"refund_id": refund.ID, "trade_no": refund.TradeNo, "reason": req.Reason,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": refund})
}

func CompleteWalletRefund(c *gin.Context) {
	var req CompleteWalletRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, errors.New("invalid wallet refund parameters"))
		return
	}
	refund, err := model.CompleteWalletRefund(req.RefundID, req.DeductQuota, req.Reason, req.ProofRef)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, refund.UserID, "user.wallet_refund_complete", map[string]any{
		"refund_id": refund.ID, "trade_no": refund.TradeNo, "deduct_quota": refund.DeductQuota,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": refund})
}

func CancelWalletRefund(c *gin.Context) {
	var req CancelWalletRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, errors.New("invalid wallet refund parameters"))
		return
	}
	if err := model.CancelWalletRefund(req.RefundID, req.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, 0, "user.wallet_refund_cancel", map[string]any{
		"refund_id": req.RefundID, "reason": req.Reason,
	})
	common.ApiSuccess(c, nil)
}
