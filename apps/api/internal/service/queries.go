package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/store"
)

// ErrNotFound alias keeps handler code uniform with the other use cases.
var queryNotFound = func() *ApplicationError {
	return NewError(404, "CALL_NOT_FOUND", "Call record was not found.")
}

// GetAccount returns the CDK summary of the session user.
func (s *Services) GetAccount(ctx context.Context, userID uuid.UUID) (store.CDKSummary, *ApplicationError) {
	summary, err := store.GetCDKSummaryForUser(ctx, s.Pool, userID)
	if err != nil {
		if err == store.ErrNotFound {
			return store.CDKSummary{}, NewError(404, "CDK_NOT_FOUND", "No CDK is bound to this account.")
		}
		slog.Error("account lookup failed", "error", err)
		return store.CDKSummary{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return summary, nil
}

// UsageReport combines quota state with call aggregates for GET /v1/usage.
type UsageReport struct {
	store.CDKSummary
	store.UsageCounters
	SuccessRate float64
}

// GetUsage computes quota totals, day/lifetime call counts and success rate.
func (s *Services) GetUsage(ctx context.Context, userID uuid.UUID) (UsageReport, *ApplicationError) {
	summary, appErr := s.GetAccount(ctx, userID)
	if appErr != nil {
		return UsageReport{}, appErr
	}
	counters, err := store.GetUsageCounters(ctx, s.Pool, userID)
	if err != nil {
		slog.Error("usage lookup failed", "error", err)
		return UsageReport{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}

	var successRate float64
	settled := counters.SuccessTotal + counters.FailedTotal
	if settled > 0 {
		successRate = float64(counters.SuccessTotal) / float64(settled)
	}
	return UsageReport{CDKSummary: summary, UsageCounters: counters, SuccessRate: successRate}, nil
}

// CallsPage is one cursor page of the user's call log.
type CallsPage struct {
	Items      []store.CallRecord
	NextCursor *store.CallCursor
}

// ListCalls returns a redacted, owner-scoped call log page. limit is clamped
// to [1,100]; the cursor is the previous page's last stable sort boundary.
func (s *Services) ListCalls(ctx context.Context, userID uuid.UUID, cursor *store.CallCursor, limit int) (CallsPage, *ApplicationError) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	records, err := store.ListCallsForUser(ctx, s.Pool, userID, cursor, limit+1)
	if err != nil {
		slog.Error("call list failed", "error", err)
		return CallsPage{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}

	page := CallsPage{Items: records}
	if len(records) > limit {
		// Drop the lookahead row and expose the final returned stable boundary.
		records = records[:limit]
		boundary := store.CallCursor{
			AcceptedAt: records[len(records)-1].AcceptedAt,
			RequestID:  records[len(records)-1].RequestID,
		}
		page.NextCursor = &boundary
		page.Items = records
	}
	return page, nil
}

// GetCall resolves one call for the owner only.
func (s *Services) GetCall(ctx context.Context, userID uuid.UUID, requestID string) (store.CallRecord, *ApplicationError) {
	record, err := store.GetCallForUser(ctx, s.Pool, userID, requestID)
	if err != nil {
		if err == store.ErrNotFound {
			return store.CallRecord{}, queryNotFound()
		}
		slog.Error("call lookup failed", "error", err)
		return store.CallRecord{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return record, nil
}
