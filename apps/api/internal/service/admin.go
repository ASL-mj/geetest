package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/store"
)

// AdminSession bundles the authenticated operator into handlers.
type AdminSession struct {
	ID       uuid.UUID
	Username string
	Role     string
}

// adminSessionCookie keeps operator sessions in a separate namespace.
const adminSessionCookie = "admin_session"

// AdminLogin verifies credentials and issues an operator session cookie.
// Login failures are deliberately indistinguishable between unknown users
// and wrong passwords.
func (s *Services) AdminLogin(ctx context.Context, username, password string) (string, *ApplicationError) {
	admin, err := store.GetAdminByUsername(ctx, s.Pool, username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", ErrAdminAuthFailed()
		}
		slog.Error("admin lookup failed", "error", err)
		return "", NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	if admin.Status != "ACTIVE" || !crypto.VerifyPassword(password, admin.PasswordHash) {
		return "", ErrAdminAuthFailed()
	}

	token, err := crypto.GenerateOpaqueToken()
	if err != nil {
		slog.Error("admin token generation failed", "error", err)
		return "", NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	sessionID := uuid.New()
	expires := nowUTC().Add(s.Settings.AdminSessionTTL)
	if err := store.CreateAdminSession(ctx, s.Pool, sessionID, admin.ID, crypto.HMACSHA256(token, s.Settings.SessionSecret), expires); err != nil {
		slog.Error("admin session create failed", "error", err)
		return "", NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	if err := store.TouchAdminLogin(ctx, s.Pool, admin.ID); err != nil {
		slog.Error("admin touch login failed", "error", err)
	}
	return token, nil
}

// ResolveAdminSession validates the operator cookie into an AdminSession.
func (s *Services) ResolveAdminSession(ctx context.Context, token string) (AdminSession, *ApplicationError) {
	admin, err := store.ResolveAdminSession(ctx, s.Pool, crypto.HMACSHA256(token, s.Settings.SessionSecret))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return AdminSession{}, ErrAdminSessionInvalid()
		}
		slog.Error("admin session lookup failed", "error", err)
		return AdminSession{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return AdminSession{ID: admin.ID, Username: admin.Username, Role: admin.Role}, nil
}

// AdminLogout revokes exactly the presented session.
func (s *Services) AdminLogout(ctx context.Context, token string) *ApplicationError {
	if err := store.RevokeAdminSession(ctx, s.Pool, crypto.HMACSHA256(token, s.Settings.SessionSecret)); err != nil {
		slog.Error("admin logout failed", "error", err)
		return NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return nil
}

// AdminBootstrapResult reports what the bootstrap wrote.
type AdminBootstrapResult struct {
	AdminID  uuid.UUID
	Username string
}

// BootstrapAdmin upserts the single bootstrap operator from settings.
func (s *Services) BootstrapAdmin(ctx context.Context) (AdminBootstrapResult, *ApplicationError) {
	if s.Settings.AdminUsername == "" || s.Settings.AdminPassword == "" {
		return AdminBootstrapResult{}, NewError(422, "INVALID_REQUEST", "ADMIN_BOOTSTRAP_USERNAME and ADMIN_BOOTSTRAP_PASSWORD are required.")
	}
	hash, err := crypto.HashPassword(s.Settings.AdminPassword)
	if err != nil {
		slog.Error("admin password hash failed", "error", err)
		return AdminBootstrapResult{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	adminID := uuid.New()
	if err := store.UpsertAdmin(ctx, s.Pool, store.AdminUser{
		ID:           adminID,
		Username:     s.Settings.AdminUsername,
		PasswordHash: hash,
		Role:         store.AdminRoleAdmin,
	}); err != nil {
		slog.Error("admin bootstrap failed", "error", err)
		return AdminBootstrapResult{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return AdminBootstrapResult{AdminID: adminID, Username: s.Settings.AdminUsername}, nil
}

// CreateCdkBatchRequest carries the operator generation inputs.
type CreateCdkBatchRequest struct {
	Name                string
	Description         string
	Quota               int64
	Count               int
	ActivationDeadline  *time.Time
	ServiceDurationDays *int
	Reason              string
}

// CreateCdkBatchResult returns the batch and the plaintext codes; they are
// also sealed for later operator re-display in CDK management.
type CreateCdkBatchResult struct {
	BatchID uuid.UUID
	Codes   []string
}

// CreateCdkBatch generates count CDKs under one audited transaction.
func (s *Services) CreateCdkBatch(ctx context.Context, admin AdminSession, request CreateCdkBatchRequest, ipMasked string) (CreateCdkBatchResult, *ApplicationError) {
	if admin.Role != store.AdminRoleAdmin {
		return CreateCdkBatchResult{}, ErrAdminForbidden()
	}
	if request.Name == "" || request.Count < 1 || request.Count > 500 ||
		request.Quota < 1 || request.Quota > 1_000_000 || request.Reason == "" {
		return CreateCdkBatchResult{}, ErrInvalidRequest()
	}
	if request.ActivationDeadline != nil && request.ServiceDurationDays == nil {
		return CreateCdkBatchResult{}, ErrInvalidRequest()
	}

	var result CreateCdkBatchResult
	batchID := uuid.New()
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		if err := store.CreateCDKBatch(ctx, q, batchID, request.Name, request.Description,
			request.Quota, request.ActivationDeadline, request.ServiceDurationDays, admin.ID); err != nil {
			return err
		}
		// Default the remark to the batch description/name so every code is
		// identifiable in CDK management; single creations pass their own.
		remark := request.Description
		if remark == "" {
			remark = request.Name
		}
		codes := make([]string, 0, request.Count)
		for i := 0; i < request.Count; i++ {
			code, err := crypto.GenerateCDKCode("CAPTCHA")
			if err != nil {
				return err
			}
			normalized := crypto.NormalizeCDK(code)
			sealed, err := crypto.Seal(s.Settings.SessionSecret, code)
			if err != nil {
				return err
			}
			var remarkPtr *string
			if remark != "" {
				remarkCopy := remark
				remarkPtr = &remarkCopy
			}
			if err := store.CreateCdk(ctx, q, batchID, normalized[:8],
				crypto.HMACSHA256(normalized, s.Settings.CDKPepper), sealed,
				request.Quota, request.ActivationDeadline, remarkPtr); err != nil {
				return err
			}
			codes = append(codes, code)
		}
		result = CreateCdkBatchResult{BatchID: batchID, Codes: codes}
		return recordAudit(ctx, q, admin, "cdk_batch.created", "cdk_batch", batchID, nil,
			map[string]any{"name": request.Name, "count": request.Count, "quota": request.Quota},
			request.Reason, ipMasked)
	})
	if err != nil {
		if appErr, ok := AsApplicationError(err); ok {
			return CreateCdkBatchResult{}, appErr
		}
		slog.Error("cdk batch creation failed", "error", err)
		return CreateCdkBatchResult{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return result, nil
}

// RevealCdkCode decrypts a CDK's sealed plaintext for operators.
func (s *Services) RevealCdkCode(ctx context.Context, cdkID uuid.UUID) (string, *ApplicationError) {
	sealed, err := store.GetCdkCiphertext(ctx, s.Pool, cdkID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", ErrCDKNotFound()
		}
		slog.Error("reveal cdk lookup failed", "error", err)
		return "", NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	code, err := crypto.Open(s.Settings.SessionSecret, sealed)
	if err != nil {
		return "", NewError(422, "CDK_CODE_UNAVAILABLE", "该 CDK 缺少可恢复的密文，无法再次显示。")
	}
	return code, nil
}

// SetCdkRemark rewrites the operator note shown in CDK management.
func (s *Services) SetCdkRemark(ctx context.Context, admin AdminSession, cdkID uuid.UUID, remark string) *ApplicationError {
	if admin.Role != store.AdminRoleAdmin {
		return ErrAdminForbidden()
	}
	if len(remark) > 200 {
		return ErrInvalidRequest()
	}
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		if err := store.UpdateCdkRemark(ctx, q, cdkID, remark); err != nil {
			return err
		}
		return recordAudit(ctx, q, admin, "cdk.remark_updated", "cdk", cdkID, nil,
			map[string]any{"remark": remark}, "更新备注", "")
	})
	if err != nil {
		slog.Error("update cdk remark failed", "error", err)
		return NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return nil
}

// SystemConfigView is the operator-facing runtime configuration snapshot.
// APIBaseURL is display-only: it is what the docs pages show as the
// platform endpoint; the underlying solver stays configured by the
// environment and is never touched from here.
type SystemConfigView struct {
	APIBaseURL  string
	HasOverride bool
}

// GetSystemConfig reports the configured display base URL, or empty when
// the docs should fall back to the current origin.
func (s *Services) GetSystemConfig(ctx context.Context) (SystemConfigView, *ApplicationError) {
	override, err := store.GetSystemSetting(ctx, s.Pool, store.SettingAPIBaseURL)
	if err == nil {
		return SystemConfigView{APIBaseURL: override, HasOverride: true}, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		slog.Error("read system config failed", "error", err)
		return SystemConfigView{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return SystemConfigView{APIBaseURL: ""}, nil
}

// PublicMeta is the anonymous configuration snapshot rendered into docs.
type PublicMeta struct {
	APIBaseURL string
}

// GetPublicMeta resolves the display base URL for unauthenticated docs.
func (s *Services) GetPublicMeta(ctx context.Context) (PublicMeta, *ApplicationError) {
	view, appErr := s.GetSystemConfig(ctx)
	if appErr != nil {
		return PublicMeta{}, appErr
	}
	return PublicMeta{APIBaseURL: view.APIBaseURL}, nil
}

// SetAPIBaseURL stores and audits the display-only base URL used by the
// documentation pages. An empty value clears the override.
func (s *Services) SetAPIBaseURL(ctx context.Context, admin AdminSession, baseURL, reason string, ipMasked string) *ApplicationError {
	if admin.Role != store.AdminRoleAdmin {
		return ErrAdminForbidden()
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
			return NewError(422, "INVALID_REQUEST", "接入地址必须是 http(s) URL。")
		}
	}
	if reason == "" {
		return ErrInvalidRequest()
	}
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		if err := store.UpsertSystemSetting(ctx, q, store.SettingAPIBaseURL, baseURL); err != nil {
			return err
		}
		return recordAudit(ctx, q, admin, "system.api_base_url_changed", "system", uuid.Nil,
			nil, map[string]any{"api_base_url": baseURL}, reason, ipMasked)
	})
	if err != nil {
		slog.Error("save api base url failed", "error", err)
		return NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return nil
}

// QuotaAdjustmentRequest is the operator payload for CDK top-ups.
type QuotaAdjustmentRequest struct {
	Delta  int64
	Reason string
}

// AdjustCdkQuota moves quota through the ledger contract and audits it.
func (s *Services) AdjustCdkQuota(ctx context.Context, admin AdminSession, cdkID uuid.UUID, request QuotaAdjustmentRequest, ipMasked string) (store.QuotaSnapshot, *ApplicationError) {
	if admin.Role != store.AdminRoleAdmin {
		return store.QuotaSnapshot{}, ErrAdminForbidden()
	}
	if request.Delta == 0 || request.Delta > 1_000_000 || request.Delta < -1_000_000 || request.Reason == "" {
		return store.QuotaSnapshot{}, ErrInvalidRequest()
	}

	var snapshot store.QuotaSnapshot
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		before, err := readQuotaForAdmin(ctx, q, cdkID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrCDKNotFound()
			}
			return err
		}
		snapshot, err = store.AdjustCDKQuota(ctx, q, cdkID, request.Delta)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrCDKNotFound()
			}
			return err
		}
		if err := store.InsertQuotaLedgerEntry(ctx, q, cdkID, before.BoundUserID, admin.ID,
			snapshot, request.Delta, store.EntryAdminAdjustment, request.Reason); err != nil {
			return err
		}
		return recordAudit(ctx, q, admin, "cdk.quota_adjusted", "cdk", cdkID,
			map[string]any{"quota_total": before.Total, "quota_remaining": before.Remaining},
			map[string]any{"quota_total": snapshot.Total, "quota_remaining": snapshot.Remaining},
			request.Reason, ipMasked)
	})
	if err != nil {
		if appErr, ok := AsApplicationError(err); ok {
			return store.QuotaSnapshot{}, appErr
		}
		slog.Error("quota adjustment failed", "error", err)
		return store.QuotaSnapshot{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return snapshot, nil
}

// SetCdkEnabled toggles DISABLED/ACTIVE on an operator-visible CDK.
func (s *Services) SetCdkEnabled(ctx context.Context, admin AdminSession, cdkID uuid.UUID, enable bool, reason, ipMasked string) (string, *ApplicationError) {
	if admin.Role != store.AdminRoleAdmin {
		return "", ErrAdminForbidden()
	}
	if reason == "" {
		return "", ErrInvalidRequest()
	}
	status := "DISABLED"
	if enable {
		status = "ACTIVE"
	}
	var before []byte
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		var previousStatus string
		var remaining int64
		err := q.QueryRow(ctx, `SELECT status, quota_remaining FROM cdks WHERE id = $1`, cdkID).
			Scan(&previousStatus, &remaining)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
				return ErrCDKNotFound()
			}
			return err
		}
		before, _ = json.Marshal(map[string]any{"status": previousStatus, "quota_remaining": remaining})

		if err := store.SetCdkStatus(ctx, q, cdkID, status); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrCDKNotFound()
			}
			return err
		}
		return recordAudit(ctx, q, admin, "cdk.status_changed", "cdk", cdkID,
			json.RawMessage(before), map[string]any{"status": status}, reason, ipMasked)
	})
	if err != nil {
		if appErr, ok := AsApplicationError(err); ok {
			return "", appErr
		}
		slog.Error("cdk status change failed", "error", err)
		return "", NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return status, nil
}

// SetUserEnabled suspends or restores a user; suspension is effective
// immediately because session resolution filters on status.
func (s *Services) SetUserEnabled(ctx context.Context, admin AdminSession, userID uuid.UUID, enable bool, reason, ipMasked string) (string, *ApplicationError) {
	if admin.Role != store.AdminRoleAdmin {
		return "", ErrAdminForbidden()
	}
	if reason == "" {
		return "", ErrInvalidRequest()
	}
	status := "SUSPENDED"
	if enable {
		status = "ACTIVE"
	}
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		if err := store.SetUserStatus(ctx, q, userID, status); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrUserNotFound()
			}
			return err
		}
		if !enable {
			if _, err := q.Exec(ctx, `
				UPDATE user_sessions SET revoked_at = now()
				WHERE user_id = $1 AND revoked_at IS NULL
			`, userID); err != nil {
				return err
			}
		}
		return recordAudit(ctx, q, admin, "user.status_changed", "user", userID,
			nil, map[string]any{"status": status}, reason, ipMasked)
	})
	if err != nil {
		if appErr, ok := AsApplicationError(err); ok {
			return "", appErr
		}
		slog.Error("user status change failed", "error", err)
		return "", NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return status, nil
}

// AdminDashboard is the landing metrics payload.
type AdminDashboard struct {
	store.DashboardCounters
	SuccessRate float64
}

// GetAdminDashboard aggregates landing metrics for operators.
func (s *Services) GetAdminDashboard(ctx context.Context) (AdminDashboard, *ApplicationError) {
	counters, err := store.GetDashboardCounters(ctx, s.Pool)
	if err != nil {
		slog.Error("dashboard aggregation failed", "error", err)
		return AdminDashboard{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	var successRate float64
	if counters.CallsToday > 0 {
		successRate = float64(counters.CallsToday-counters.CallsFailedToday) / float64(counters.CallsToday)
	}
	return AdminDashboard{DashboardCounters: counters, SuccessRate: successRate}, nil
}

// recordAudit appends one audit row inside the caller's transaction; before
// and after snapshots are optional.
func recordAudit(ctx context.Context, q store.Querier, admin AdminSession, action, targetType string, targetID uuid.UUID, before, after any, reason, ipMasked string) error {
	toRaw := func(value any) []byte {
		if value == nil {
			return nil
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		return raw
	}
	return store.InsertAdminAuditLog(ctx, q, store.AdminAuditLog{
		ID:          uuid.New(),
		AdminUserID: admin.ID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		BeforeJSON:  toRaw(before),
		AfterJSON:   toRaw(after),
		Reason:      reason,
		IPMasked:    ipMasked,
	})
}

// adminCdkSnapshot pairs quota counters with the bound user for the ledger.
type adminCdkSnapshot struct {
	store.QuotaSnapshot
	BoundUserID *uuid.UUID
}

func readQuotaForAdmin(ctx context.Context, q store.Querier, cdkID uuid.UUID) (adminCdkSnapshot, error) {
	var snapshot adminCdkSnapshot
	err := q.QueryRow(ctx, `
		SELECT quota_remaining, quota_used, quota_reserved, quota_total, bound_user_id
		FROM cdks WHERE id = $1
	`, cdkID).Scan(&snapshot.Remaining, &snapshot.Used, &snapshot.Reserved, &snapshot.Total, &snapshot.BoundUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return adminCdkSnapshot{}, store.ErrNotFound
		}
		return adminCdkSnapshot{}, err
	}
	return snapshot, nil
}
