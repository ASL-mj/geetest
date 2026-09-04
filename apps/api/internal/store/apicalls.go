package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// API call lifecycle states per the V1 state model.
const (
	CallStatusReceived       = "RECEIVED"
	CallStatusRejected       = "REJECTED"
	CallStatusReserved       = "RESERVED"
	CallStatusDispatched     = "DISPATCHED"
	CallStatusSucceeded      = "SUCCEEDED"
	CallStatusFailedRefunded = "FAILED_REFUNDED"
)

// APICall is the persisted record of one platform invocation.
type APICall struct {
	ID                   uuid.UUID
	RequestID            string
	Operation            string
	IdempotencyKeyHash   []byte
	UserID               uuid.UUID
	CDKID                uuid.UUID
	APIKeyID             uuid.UUID
	APIKeyNameSnapshot   string
	APIKeyPrefixSnapshot string
	CaptchaID            string
	RiskType             string
	Status               string
	HTTPStatus           int
	ErrorCode            *string
	ErrorSummary         *string
	AcceptedAt           time.Time
	CompletedAt          *time.Time
	DurationMS           *int
	QuotaReserved        bool
	QuotaRefunded        bool
	ResponseJSON         json.RawMessage
	ClientIPMasked       string
	ClientIPHash         []byte
	UserAgent            string
}

const apiCallColumns = `
	id, request_id, operation, idempotency_key_hash, user_id, cdk_id, api_key_id,
	api_key_name_snapshot, api_key_prefix_snapshot, captcha_id, risk_type, status,
	http_status, error_code, error_summary, accepted_at, completed_at, duration_ms,
	quota_reserved, quota_refunded, response_json
`

func scanAPICall(row pgx.Row) (APICall, error) {
	var call APICall
	err := row.Scan(
		&call.ID, &call.RequestID, &call.Operation, &call.IdempotencyKeyHash, &call.UserID, &call.CDKID, &call.APIKeyID,
		&call.APIKeyNameSnapshot, &call.APIKeyPrefixSnapshot, &call.CaptchaID, &call.RiskType, &call.Status,
		&call.HTTPStatus, &call.ErrorCode, &call.ErrorSummary, &call.AcceptedAt, &call.CompletedAt, &call.DurationMS,
		&call.QuotaReserved, &call.QuotaRefunded, &call.ResponseJSON,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return APICall{}, ErrNotFound
		}
		return APICall{}, err
	}
	return call, nil
}

// CreateAPICall inserts the RECEIVED record; the (user_id, operation,
// idempotency_key_hash) unique constraint is the durable idempotency guard.
// It returns ErrUniqueCall when the same key already exists.
var ErrUniqueCall = errors.New("idempotent call already exists")

func CreateAPICall(ctx context.Context, q Querier, call APICall) error {
	_, err := q.Exec(ctx, `
		INSERT INTO api_calls (id, request_id, operation, idempotency_key_hash, user_id, cdk_id, api_key_id,
		                      api_key_name_snapshot, api_key_prefix_snapshot, captcha_id, risk_type,
		                      status, http_status, quota_reserved, quota_refunded,
		                      client_ip_masked, client_ip_hash, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 0, false, false, $13, $14, $15)
	`, call.ID, call.RequestID, call.Operation, call.IdempotencyKeyHash, call.UserID, call.CDKID, call.APIKeyID,
		call.APIKeyNameSnapshot, call.APIKeyPrefixSnapshot, call.CaptchaID, call.RiskType,
		CallStatusReceived, call.ClientIPMasked, call.ClientIPHash, call.UserAgent)
	if IsUniqueViolation(err) {
		return ErrUniqueCall
	}
	return err
}

// GetAPICallByIdempotency resolves a prior call for replay or 409 handling.
func GetAPICallByIdempotency(ctx context.Context, q Querier, userID uuid.UUID, operation string, keyHash []byte) (APICall, error) {
	row := q.QueryRow(ctx, `
		SELECT `+apiCallColumns+`
		FROM api_calls
		WHERE user_id = $1 AND operation = $2 AND idempotency_key_hash = $3
	`, userID, operation, keyHash)
	return scanAPICall(row)
}

// GetAPICallByRequestID resolves one call by its platform request id.
func GetAPICallByRequestID(ctx context.Context, q Querier, requestID string) (APICall, error) {
	row := q.QueryRow(ctx, `
		SELECT `+apiCallColumns+`
		FROM api_calls
		WHERE request_id = $1
	`, requestID)
	return scanAPICall(row)
}

// RejectAPICall finalizes a call that never reached the solver.
func RejectAPICall(ctx context.Context, q Querier, callID uuid.UUID, httpStatus int, errorCode, errorSummary string, duration time.Duration) error {
	_, err := q.Exec(ctx, `
		UPDATE api_calls
		SET status = $2, http_status = $3, error_code = $4, error_summary = $5,
		    completed_at = now(), duration_ms = $6
		WHERE id = $1
	`, callID, CallStatusRejected, httpStatus, errorCode, errorSummary, duration.Milliseconds())
	return err
}

// ReserveAPICall moves the call to RESERVED after quota was pre-deducted.
func ReserveAPICall(ctx context.Context, q Querier, callID uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE api_calls SET status = $2, quota_reserved = true WHERE id = $1`,
		callID, CallStatusReserved)
	return err
}

// DispatchAPICall marks that the request was handed to the solver.
func DispatchAPICall(ctx context.Context, q Querier, callID uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE api_calls SET status = $2 WHERE id = $1`, callID, CallStatusDispatched)
	return err
}

// CompleteAPICallSuccess stores the terminal success state with the response.
func CompleteAPICallSuccess(ctx context.Context, q Querier, callID uuid.UUID, responseJSON json.RawMessage, duration time.Duration) error {
	_, err := q.Exec(ctx, `
		UPDATE api_calls
		SET status = $2, http_status = 200, completed_at = now(), duration_ms = $3, response_json = $4
		WHERE id = $1
	`, callID, CallStatusSucceeded, duration.Milliseconds(), responseJSON)
	return err
}

// CompleteAPICallFailure stores the terminal failure state; refunded reports
// whether the quota refund succeeded (must be exactly once).
func CompleteAPICallFailure(ctx context.Context, q Querier, callID uuid.UUID, httpStatus int, errorCode, errorSummary string, refunded bool, duration time.Duration) error {
	_, err := q.Exec(ctx, `
		UPDATE api_calls
		SET status = $2, http_status = $3, error_code = $4, error_summary = $5,
		    quota_refunded = $6, completed_at = now(), duration_ms = $7
		WHERE id = $1
	`, callID, CallStatusFailedRefunded, httpStatus, errorCode, errorSummary, refunded, duration.Milliseconds())
	return err
}
