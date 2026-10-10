package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

const (
	AbuseReviewStatusPending       = "pending"
	AbuseReviewStatusResolved      = "resolved"
	AbuseReviewStatusFalsePositive = "false_positive"
	AbuseReviewScoreVersion        = "abuse-score-v1"
	AbuseReviewWindow              = 10 * time.Minute
	AbuseReviewThresholdReview     = 3
	AbuseReviewThresholdManual     = 6
)

var abuseSignalWeights = map[string]int{
	"multi_ip":       1,
	"multi_token":    2,
	"token_churn":    2,
	"model_switch":   1,
	"retry_storm":    2,
	"provider_error": 2,
	"failure_burst":  2,
	"long_request":   1,
	"long_stream":    1,
	"keyword_match":  1,
	"content_audit":  2,
}

// AbuseReview is the operator-owned review queue for one shadow signal.
// Evidence is already redacted by the signal recorder and contains no prompt,
// bearer token, provider secret, or raw IP address.
type AbuseReview struct {
	Id           int    `json:"id"`
	EventId      string `json:"event_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId       int    `json:"user_id" gorm:"index"`
	TokenId      int    `json:"token_id" gorm:"index"`
	Action       string `json:"action" gorm:"type:varchar(128);index"`
	Status       string `json:"status" gorm:"type:varchar(20);index"`
	RiskScore    int    `json:"risk_score"`
	Disposition  string `json:"disposition" gorm:"-"`
	ScoreVersion string `json:"score_version" gorm:"type:varchar(32)"`
	EvidenceJSON string `json:"evidence_json" gorm:"type:text"`
	ReviewerId   int    `json:"reviewer_id" gorm:"index"`
	ReviewNote   string `json:"review_note" gorm:"type:text"`
	CreatedAt    int64  `json:"created_at" gorm:"index"`
	UpdatedAt    int64  `json:"updated_at"`
	ResolvedAt   int64  `json:"resolved_at"`
}

func (AbuseReview) TableName() string { return "abuse_reviews" }

func AbuseSignalWeight(kind string) int {
	kind = strings.TrimPrefix(kind, "abuse.")
	return abuseSignalWeights[kind]
}

func AbuseSignalScore(kind string, fields AuditFields) int {
	score := AbuseSignalWeight(kind)
	if score == 0 {
		return 0
	}
	if kind == "retry_storm" && numericField(fields, "attempts") >= 5 {
		score++
	}
	if kind == "multi_ip" && numericField(fields, "distinct_count") >= 4 {
		score++
	}
	if kind == "multi_token" && numericField(fields, "distinct_count") >= 4 {
		score++
	}
	return score
}

// AbuseDisposition is an operator priority only. It never changes account,
// token, wallet, or provider state.
func AbuseDisposition(score int) string {
	switch {
	case score >= AbuseReviewThresholdManual:
		return "manual_action"
	case score >= AbuseReviewThresholdReview:
		return "review"
	default:
		return "observe"
	}
}

// AbuseAggregateScore returns the score for distinct signals observed for the
// same user and token in the current review window. Repeated records of one
// action do not inflate the score, which keeps retry noise from becoming an
// automatic sanction.
func AbuseAggregateScore(userID, tokenID int, kind string, fields AuditFields) int {
	score := AbuseSignalScore(kind, fields)
	if DB == nil || userID <= 0 || tokenID <= 0 {
		return score
	}
	cutoff := common.GetTimestamp() - int64(AbuseReviewWindow/time.Second)
	var actions []string
	if err := DB.Model(&AbuseReview{}).
		Where("user_id = ? AND token_id = ? AND created_at >= ?", userID, tokenID, cutoff).
		Distinct("action").Pluck("action", &actions).Error; err != nil {
		return score
	}
	seen := map[string]bool{}
	for _, action := range actions {
		if strings.TrimPrefix(action, "abuse.") == strings.TrimPrefix(kind, "abuse.") {
			continue
		}
		if seen[action] {
			continue
		}
		seen[action] = true
		score += AbuseSignalWeight(action)
	}
	return score
}

func numericField(fields AuditFields, key string) int {
	if fields == nil {
		return 0
	}
	switch value := fields[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func ValidAbuseReviewStatus(status string) bool {
	return status == AbuseReviewStatusPending || status == AbuseReviewStatusResolved || status == AbuseReviewStatusFalsePositive
}

func ValidAbuseReviewTransition(status string) bool {
	return status == AbuseReviewStatusResolved || status == AbuseReviewStatusFalsePositive
}

func CreateAbuseReview(review *AbuseReview) error {
	if review == nil || review.EventId == "" || review.UserId <= 0 || review.TokenId <= 0 || review.Action == "" {
		return errors.New("invalid abuse review")
	}
	if !ValidAbuseReviewStatus(review.Status) {
		return errors.New("invalid abuse review status")
	}
	if review.ScoreVersion == "" {
		review.ScoreVersion = AbuseReviewScoreVersion
	}
	if review.CreatedAt == 0 {
		review.CreatedAt = common.GetTimestamp()
	}
	if review.UpdatedAt == 0 {
		review.UpdatedAt = review.CreatedAt
	}
	review.Disposition = AbuseDisposition(review.RiskScore)
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(review)
	return result.Error
}

func ListAbuseReviews(status, action string, start, limit int) ([]*AbuseReview, int64, error) {
	query := DB.Model(&AbuseReview{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*AbuseReview, 0)
	if err := query.Order("created_at DESC").Order("id DESC").Offset(start).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	for _, item := range items {
		item.Disposition = AbuseDisposition(item.RiskScore)
	}
	return items, total, nil
}

// AbuseReviewSummary is a read-only operator snapshot. It contains counts only;
// no prompt, token, IP, or provider credential is included.
type AbuseReviewSummary struct {
	Total          int64 `json:"total"`
	Pending        int64 `json:"pending"`
	Resolved       int64 `json:"resolved"`
	FalsePositive  int64 `json:"false_positive"`
	Last24Hours    int64 `json:"last_24_hours"`
	ReviewPriority int64 `json:"review_priority"`
	ManualPriority int64 `json:"manual_priority"`
	ContentAudit   int64 `json:"content_audit"`
	KeywordMatch   int64 `json:"keyword_match"`
	GeneratedAt    int64 `json:"generated_at"`
}

func GetAbuseReviewSummary() (AbuseReviewSummary, error) {
	if DB == nil {
		return AbuseReviewSummary{}, errors.New("abuse review database is unavailable")
	}
	var summary AbuseReviewSummary
	count := func(destination *int64, query string, args ...any) error {
		return DB.Model(&AbuseReview{}).Where(query, args...).Count(destination).Error
	}
	if err := DB.Model(&AbuseReview{}).Count(&summary.Total).Error; err != nil {
		return AbuseReviewSummary{}, err
	}
	for status, destination := range map[string]*int64{
		AbuseReviewStatusPending:       &summary.Pending,
		AbuseReviewStatusResolved:      &summary.Resolved,
		AbuseReviewStatusFalsePositive: &summary.FalsePositive,
	} {
		if err := count(destination, "status = ?", status); err != nil {
			return AbuseReviewSummary{}, err
		}
	}
	cutoff := common.GetTimestamp() - int64((24*time.Hour)/time.Second)
	if err := count(&summary.Last24Hours, "created_at >= ?", cutoff); err != nil {
		return AbuseReviewSummary{}, err
	}
	if err := count(&summary.ReviewPriority, "risk_score >= ?", AbuseReviewThresholdReview); err != nil {
		return AbuseReviewSummary{}, err
	}
	if err := count(&summary.ManualPriority, "risk_score >= ?", AbuseReviewThresholdManual); err != nil {
		return AbuseReviewSummary{}, err
	}
	if err := count(&summary.ContentAudit, "action = ?", "abuse.content_audit"); err != nil {
		return AbuseReviewSummary{}, err
	}
	if err := count(&summary.KeywordMatch, "action = ?", "abuse.keyword_match"); err != nil {
		return AbuseReviewSummary{}, err
	}
	summary.GeneratedAt = common.GetTimestamp()
	return summary, nil
}

func GetAbuseReview(id int) (*AbuseReview, error) {
	var review AbuseReview
	if err := DB.First(&review, id).Error; err != nil {
		return nil, err
	}
	return &review, nil
}

func UpdateAbuseReview(review *AbuseReview) error {
	if review == nil || review.Id <= 0 || !ValidAbuseReviewStatus(review.Status) {
		return errors.New("invalid abuse review update")
	}
	review.Disposition = AbuseDisposition(review.RiskScore)
	return DB.Model(&AbuseReview{}).Where("id = ?", review.Id).Updates(map[string]any{
		"status":      review.Status,
		"reviewer_id": review.ReviewerId,
		"review_note": review.ReviewNote,
		"updated_at":  review.UpdatedAt,
		"resolved_at": review.ResolvedAt,
	}).Error
}
