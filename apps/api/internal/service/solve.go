package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/ratelimit"
	"github.com/captchaflow/service-platform/api/internal/solver"
	"github.com/captchaflow/service-platform/api/internal/store"
)

// SolveError is a typed solve outcome the transport renders directly.
type SolveError struct {
	HTTPStatus int
	Code       string
	Message    string
	RetryAfter time.Duration
}

// ErrConflictReplay marks an in-flight idempotent duplicate (409).
var ErrConflictReplay = &SolveError{HTTPStatus: 409, Code: "IDEMPOTENCY_IN_PROGRESS", Message: "An identical request is already in progress."}

// SolveOutcome is the result of one admitted call.
type SolveOutcome struct {
	RequestID  string
	Replayed   bool
	Result     *solver.SolveResult
	SolveError *SolveError
}

// SolveGateway is the port through which the solve service reaches the solver.
type SolveGateway interface {
	Solve(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error)
}

// SolveService orchestrates the V1 admission chain:
// idempotency -> rate -> concurrency -> quota reserve -> solver -> settle.
// It owns no HTTP concerns; the transport renders SolveOutcome.
type SolveService struct {
	pool      store.Querier
	gateway   SolveGateway
	limiter   ratelimit.Limiter
	operation string
	pepper    string
}

// NewSolveService wires the orchestration dependencies. The pepper scopes
// idempotency-key hashing; any server-side pepper works because scoping to
// the user and operation comes from the api_calls unique constraint.
func NewSolveService(pool store.Querier, gateway SolveGateway, limiter ratelimit.Limiter, pepper string) *SolveService {
	return &SolveService{pool: pool, gateway: gateway, limiter: limiter, operation: "captcha.solve", pepper: pepper}
}

// SolveInput carries the authenticated caller plus the validated request.
type SolveInput struct {
	Caller         AuthenticatedCaller
	CaptchaID      string
	IdempotencyKey string
	ClientIPMasked string
	ClientIPHash   []byte
	UserAgent      string
}

// Solve runs the full admission chain. The gateway call happens outside any
// database transaction; ledger writes own their transactions.
func (s *SolveService) Solve(ctx context.Context, input SolveInput) SolveOutcome {
	caller := input.Caller

	keyHash := crypto.HMACSHA256(input.IdempotencyKey, s.pepper)
	existing, err := store.GetAPICallByIdempotency(ctx, s.pool, caller.UserID, s.operation, keyHash)
	switch {
	case err == nil:
		return s.replayOutcome(existing)
	case errors.Is(err, store.ErrNotFound):
		// Fresh call: continue below.
	default:
		slog.Error("idempotency lookup failed", "error", err)
		return SolveOutcome{SolveError: &SolveError{HTTPStatus: 500, Code: "INTERNAL_ERROR", Message: "Internal server error."}}
	}

	requestID := crypto.NewRequestID()
	callID := uuid.New()
	record := store.APICall{
		ID:                   callID,
		RequestID:            requestID,
		Operation:            s.operation,
		IdempotencyKeyHash:   keyHash,
		UserID:               caller.UserID,
		CDKID:                caller.CDKID,
		APIKeyID:             caller.APIKeyID,
		APIKeyNameSnapshot:   caller.APIKey.Name,
		APIKeyPrefixSnapshot: caller.APIKey.KeyPrefix,
		CaptchaID:            input.CaptchaID,
		RiskType:             "slide",
		ClientIPMasked:       input.ClientIPMasked,
		ClientIPHash:         input.ClientIPHash,
		UserAgent:            input.UserAgent,
	}
	if err := store.CreateAPICall(ctx, s.pool, record); err != nil {
		if errors.Is(err, store.ErrUniqueCall) {
			// Lost the insert race with a duplicate: replay that row.
			if winner, lookupErr := store.GetAPICallByIdempotency(ctx, s.pool, caller.UserID, s.operation, keyHash); lookupErr == nil {
				return s.replayOutcome(winner)
			}
			return SolveOutcome{RequestID: requestID, SolveError: ErrConflictReplay}
		}
		slog.Error("create api call failed", "error", err)
		return SolveOutcome{SolveError: &SolveError{HTTPStatus: 500, Code: "INTERNAL_ERROR", Message: "Internal server error."}}
	}

	// Rate limit (per CDK per minute); Redis failure fails open.
	allowed, retryAfter, err := s.limiter.AllowRate(ctx, caller.CDKID.String())
	if err != nil {
		retryAfter = 0
	}
	if !allowed {
		return s.reject(ctx, record, 429, "RATE_LIMITED", "Rate limit exceeded.", retryAfter)
	}

	// Concurrency lease (per CDK), always released when Solve returns.
	acquired, err := s.limiter.AcquireConc(ctx, caller.CDKID.String(), requestID)
	if err != nil {
		acquired = true
	}
	if !acquired {
		return s.reject(ctx, record, 429, "CONCURRENCY_LIMITED", "Concurrency limit exceeded.", 0)
	}
	defer func() {
		_ = s.limiter.ReleaseConc(context.Background(), caller.CDKID.String(), requestID)
	}()

	// Quota pre-deduction with RESERVE ledger row; the conditional UPDATE is
	// the admission gate so exhausted CDKs never reach the solver.
	if err := store.ReserveQuota(ctx, s.pool, caller.CDKID, caller.UserID, callID, requestID, "solve reserve"); err != nil {
		if errors.Is(err, store.ErrQuotaExhausted) {
			return s.reject(ctx, record, 402, "QUOTA_EXHAUSTED", "No remaining quota.", 0)
		}
		slog.Error("quota reserve failed", "error", err)
		return s.reject(ctx, record, 500, "INTERNAL_ERROR", "Internal server error.", 0)
	}
	if err := store.ReserveAPICall(ctx, s.pool, callID); err != nil {
		slog.Error("mark reserved failed", "request_id", requestID, "error", err)
	}
	if err := store.DispatchAPICall(ctx, s.pool, callID); err != nil {
		slog.Error("mark dispatched failed", "request_id", requestID, "error", err)
	}

	// Solver call strictly outside DB transactions.
	started := time.Now()
	result, solveErr := s.gateway.Solve(ctx, requestID, solver.SolveRequest{CaptchaID: input.CaptchaID, RiskType: "slide"})
	duration := time.Since(started)

	if solveErr != nil {
		return s.settleFailure(ctx, record, solveErr, duration)
	}

	responseJSON, err := json.Marshal(map[string]any{
		"captcha_id":     result.CaptchaID,
		"lot_number":     result.LotNumber,
		"captcha_output": result.CaptchaOutput,
		"pass_token":     result.PassToken,
		"gen_time":       result.GenTime,
	})
	if err != nil {
		responseJSON = json.RawMessage(`{}`)
	}

	if err := store.ConfirmQuota(ctx, s.pool, caller.CDKID, caller.UserID, callID, requestID, "solve confirm"); err != nil {
		// Success is already committed toward the user; ledger inconsistency
		// is logged for reconciliation rather than failing the response.
		slog.Error("quota confirm failed", "request_id", requestID, "error", err)
	}
	if err := store.CompleteAPICallSuccess(ctx, s.pool, callID, duration); err != nil {
		slog.Error("complete success failed", "request_id", requestID, "error", err)
	}
	if err := store.SaveIdempotencyResponse(ctx, s.pool, callID, caller.UserID, responseJSON); err != nil {
		slog.Error("save idempotency response failed", "request_id", requestID, "error", err)
	}
	return SolveOutcome{RequestID: requestID, Result: result}
}

// replayOutcome serves a stored terminal response or flags in-progress.
func (s *SolveService) replayOutcome(existing store.APICall) SolveOutcome {
	if existing.Status == store.CallStatusSucceeded {
		payload, err := store.GetIdempotencyResponse(context.Background(), s.pool, existing.ID)
		if err == nil {
			var result solver.SolveResult
			if json.Unmarshal(payload, &result) == nil && result.CaptchaOutput != "" {
				return SolveOutcome{RequestID: existing.RequestID, Replayed: true, Result: &result}
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			slog.Error("idempotency response lookup failed", "request_id", existing.RequestID, "error", err)
		}
	}
	// RECEIVED/RESERVED/DISPATCHED or unusable payload: still executing.
	return SolveOutcome{RequestID: existing.RequestID, SolveError: ErrConflictReplay}
}

// reject finalizes a call that never reserved quota.
func (s *SolveService) reject(ctx context.Context, record store.APICall, httpStatus int, code, message string, retryAfter time.Duration) SolveOutcome {
	if err := store.RejectAPICall(ctx, s.pool, record.ID, httpStatus, code, message, 0); err != nil {
		slog.Error("reject call failed", "request_id", record.RequestID, "error", err)
	}
	return SolveOutcome{
		RequestID: record.RequestID,
		SolveError: &SolveError{
			HTTPStatus: httpStatus,
			Code:       code,
			Message:    message,
			RetryAfter: retryAfter,
		},
	}
}

// settleFailure refunds the reserved unit exactly once and persists the
// failure. The ledger unique constraint makes repeated refunds a no-op.
func (s *SolveService) settleFailure(ctx context.Context, record store.APICall, solveErr error, duration time.Duration) SolveOutcome {
	httpStatus, code, message := mapSolverError(solveErr)

	refundErr := store.RefundQuota(ctx, s.pool, record.CDKID, record.UserID, record.ID, record.RequestID, "solve refund")
	refunded := refundErr == nil
	if refundErr != nil {
		slog.Error("quota refund failed", "request_id", record.RequestID, "error", refundErr)
	}
	if err := store.CompleteAPICallFailure(ctx, s.pool, record.ID, httpStatus, code, message, refunded, duration); err != nil {
		slog.Error("complete failure failed", "request_id", record.RequestID, "error", err)
	}
	return SolveOutcome{
		RequestID: record.RequestID,
		SolveError: &SolveError{
			HTTPStatus: httpStatus,
			Code:       code,
			Message:    message,
		},
	}
}

// mapSolverError renders gateway failures into stable platform codes.
func mapSolverError(err error) (int, string, string) {
	switch {
	case errors.Is(err, solver.ErrTimeout):
		return 504, "SOLVER_TIMEOUT", "The solver did not respond in time."
	case errors.Is(err, solver.ErrUnreach):
		return 502, "SOLVER_FAILED", "The solver is unreachable."
	case errors.Is(err, solver.ErrSolver5xx):
		return 502, "SOLVER_FAILED", "The solver failed to process the request."
	case errors.Is(err, solver.ErrSolver422):
		return 422, "INVALID_REQUEST", "The solver rejected the request parameters."
	case errors.Is(err, solver.ErrSolverBad):
		return 502, "SOLVER_FAILED", "The solver returned an unusable response."
	default:
		return 502, "SOLVER_FAILED", "The solver failed to process the request."
	}
}
