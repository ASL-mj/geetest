package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Admin roles per the V1 RBAC contract.
const (
	AdminRoleAdmin  = "admin"
	AdminRoleViewer = "viewer"
)

// AdminUser is one operator identity; passwords are Argon2id hashes.
type AdminUser struct {
	ID           uuid.UUID
	Username     string
	PasswordHash string
	Role         string
	Status       string
	LastLoginAt  *time.Time
}

// GetAdminByUsername resolves an operator by unique username.
func GetAdminByUsername(ctx context.Context, q Querier, username string) (AdminUser, error) {
	var admin AdminUser
	err := q.QueryRow(ctx, `
		SELECT id, username::text, password_hash, role, status, last_login_at
		FROM admin_users WHERE username = $1
	`, username).Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &admin.Role, &admin.Status, &admin.LastLoginAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AdminUser{}, ErrNotFound
		}
		return AdminUser{}, err
	}
	return admin, nil
}

// GetAdminByID resolves an operator by session join.
func GetAdminByID(ctx context.Context, q Querier, id uuid.UUID) (AdminUser, error) {
	var admin AdminUser
	err := q.QueryRow(ctx, `
		SELECT id, username::text, password_hash, role, status, last_login_at
		FROM admin_users WHERE id = $1
	`, id).Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &admin.Role, &admin.Status, &admin.LastLoginAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AdminUser{}, ErrNotFound
		}
		return AdminUser{}, err
	}
	return admin, nil
}

// CountAdmins reports how many operator accounts exist; startup bootstrap
// only runs when this is zero so re-runs never overwrite operator passwords.
func CountAdmins(ctx context.Context, q Querier) (int, error) {
	var count int
	err := q.QueryRow(ctx, `SELECT count(*) FROM admin_users`).Scan(&count)
	return count, err
}

// UpsertAdmin creates or refreshes the bootstrap operator; the password hash
// is only replaced on re-bootstrap.
func UpsertAdmin(ctx context.Context, q Querier, admin AdminUser) error {
	_, err := q.Exec(ctx, `
		INSERT INTO admin_users (id, username, password_hash, role, status)
		VALUES ($1, $2, $3, $4, 'ACTIVE')
		ON CONFLICT (username) DO UPDATE
		SET password_hash = EXCLUDED.password_hash, role = EXCLUDED.role, updated_at = now()
	`, admin.ID, admin.Username, admin.PasswordHash, admin.Role)
	return err
}

// TouchAdminLogin records the successful login time.
func TouchAdminLogin(ctx context.Context, q Querier, adminID uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE admin_users SET last_login_at = now() WHERE id = $1`, adminID)
	return err
}

// AdminSession mirrors user_sessions with a separate cookie namespace.
func CreateAdminSession(ctx context.Context, q Querier, id, adminID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO admin_sessions (id, admin_user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, id, adminID, tokenHash, expiresAt)
	return err
}

// ResolveAdminSession validates the cookie token hash into an operator.
func ResolveAdminSession(ctx context.Context, q Querier, tokenHash []byte) (AdminUser, error) {
	var admin AdminUser
	err := q.QueryRow(ctx, `
		SELECT a.id, a.username::text, a.password_hash, a.role, a.status, a.last_login_at
		FROM admin_sessions s
		JOIN admin_users a ON a.id = s.admin_user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
		  AND a.status = 'ACTIVE'
	`, tokenHash).Scan(&admin.ID, &admin.Username, &admin.PasswordHash, &admin.Role, &admin.Status, &admin.LastLoginAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AdminUser{}, ErrNotFound
		}
		return AdminUser{}, err
	}
	return admin, nil
}

// RevokeAdminSession ends one operator session.
func RevokeAdminSession(ctx context.Context, q Querier, tokenHash []byte) error {
	_, err := q.Exec(ctx, `
		UPDATE admin_sessions SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	return err
}

// CdkBatchSummary is the batch row plus generation counts.
type CdkBatchSummary struct {
	ID                  uuid.UUID
	Name                string
	Description         *string
	DefaultQuota        int64
	ActivationDeadline  *time.Time
	ServiceDurationDays *int
	CreatedBy           *uuid.UUID
	CreatedAt           time.Time
	TotalCDKs           int64
	ActiveCDKs          int64
}

// CreateCDKBatch inserts the batch row inside the generation transaction.
func CreateCDKBatch(ctx context.Context, q Querier, id uuid.UUID, name, description string, defaultQuota int64, activationDeadline *time.Time, serviceDurationDays *int, createdBy uuid.UUID) error {
	_, err := q.Exec(ctx, `
		INSERT INTO cdk_batches (id, name, description, default_quota, activation_deadline, service_duration_days, created_by)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7)
	`, id, name, description, defaultQuota, activationDeadline, serviceDurationDays, createdBy)
	return err
}

// ListCdkBatches returns the newest batches with CDK counts.
func ListCdkBatches(ctx context.Context, q Querier, limit int) ([]CdkBatchSummary, error) {
	rows, err := q.Query(ctx, `
		SELECT b.id, b.name, b.description, b.default_quota, b.activation_deadline,
		       b.service_duration_days, b.created_by, b.created_at,
		       count(c.id), count(c.id) FILTER (WHERE c.status = 'ACTIVE')
		FROM cdk_batches b
		LEFT JOIN cdks c ON c.batch_id = b.id
		GROUP BY b.id
		ORDER BY b.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	batches := make([]CdkBatchSummary, 0, limit)
	for rows.Next() {
		var batch CdkBatchSummary
		if err := rows.Scan(&batch.ID, &batch.Name, &batch.Description, &batch.DefaultQuota,
			&batch.ActivationDeadline, &batch.ServiceDurationDays, &batch.CreatedBy, &batch.CreatedAt,
			&batch.TotalCDKs, &batch.ActiveCDKs); err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}
	return batches, rows.Err()
}

// CreateCdk inserts one generated CDK in UNACTIVATED state. The AES-GCM
// ciphertext keeps the plaintext recoverable for operators; expiry is
// computed at activation from the batch service duration.
func CreateCdk(ctx context.Context, q Querier, batchID uuid.UUID, codePrefix string, codeHash, codeCiphertext []byte, quota int64, activationDeadline *time.Time, remark *string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO cdks (id, batch_id, code_prefix, code_hash, code_ciphertext, remark, status,
		                  activation_deadline, quota_total, quota_used, quota_reserved, quota_remaining)
		VALUES ($1, $2, $3, $4, $5, $6, 'UNACTIVATED', $7, $8, 0, 0, $8)
	`, uuid.New(), batchID, codePrefix, codeHash, codeCiphertext, remark, activationDeadline, quota)
	return err
}

// UpdateCdkRemark rewrites the operator note on one CDK.
func UpdateCdkRemark(ctx context.Context, q Querier, cdkID uuid.UUID, remark string) error {
	_, err := q.Exec(ctx, `UPDATE cdks SET remark = $2, updated_at = now() WHERE id = $1`, cdkID, remark)
	return err
}

// AdminCdk is the operator-facing CDK row; hashes stay out of responses.
type AdminCdk struct {
	ID             uuid.UUID
	BatchID        uuid.UUID
	CodePrefix     string
	Status         string
	BoundUserID    *uuid.UUID
	BoundUserState *string
	Remark         *string
	BatchName      string
	ExpiresAt      *time.Time
	QuotaTotal     int64
	QuotaUsed      int64
	QuotaReserved  int64
	QuotaRemaining int64
	ActivatedAt    *time.Time
	CreatedAt      time.Time
}

// ListCdks returns operator-visible CDK rows, newest first, with the batch
// name and bound-user status joined for the merged management view.
func ListCdks(ctx context.Context, q Querier, batchID *uuid.UUID, status string, limit int) ([]AdminCdk, error) {
	sql := `
		SELECT c.id, c.batch_id, c.code_prefix, c.status, c.bound_user_id, u.status, c.remark,
		       COALESCE(b.name, ''), c.expires_at,
		       c.quota_total, c.quota_used, c.quota_reserved, c.quota_remaining, c.activated_at, c.created_at
		FROM cdks c
		LEFT JOIN cdk_batches b ON b.id = c.batch_id
		LEFT JOIN users u ON u.id = c.bound_user_id
		WHERE true`
	args := []any{}
	if batchID != nil {
		args = append(args, *batchID)
		sql += ` AND c.batch_id = $` + itoa(len(args))
	}
	if status != "" {
		args = append(args, status)
		sql += ` AND c.status = $` + itoa(len(args))
	}
	args = append(args, limit)
	sql += ` ORDER BY created_at DESC LIMIT $` + itoa(len(args))

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cdks := make([]AdminCdk, 0, limit)
	for rows.Next() {
		var cdk AdminCdk
		if err := rows.Scan(&cdk.ID, &cdk.BatchID, &cdk.CodePrefix, &cdk.Status, &cdk.BoundUserID,
			&cdk.BoundUserState, &cdk.Remark, &cdk.BatchName, &cdk.ExpiresAt,
			&cdk.QuotaTotal, &cdk.QuotaUsed, &cdk.QuotaReserved, &cdk.QuotaRemaining,
			&cdk.ActivatedAt, &cdk.CreatedAt); err != nil {
			return nil, err
		}
		cdks = append(cdks, cdk)
	}
	return cdks, rows.Err()
}

// GetCdkCiphertext loads the sealed CDK code for operator re-display.
func GetCdkCiphertext(ctx context.Context, q Querier, cdkID uuid.UUID) ([]byte, error) {
	var sealed []byte
	err := q.QueryRow(ctx, `SELECT code_ciphertext FROM cdks WHERE id = $1`, cdkID).Scan(&sealed)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return sealed, nil
}

// System settings: simple string overrides applied at runtime.
const SettingAPIBaseURL = "api_base_url"

// GetSystemSetting returns one stored override (value_json holds a plain
// JSON string), or ErrNotFound.
func GetSystemSetting(ctx context.Context, q Querier, key string) (string, error) {
	var value string
	err := q.QueryRow(ctx, `SELECT value_json #>> '{}' FROM system_settings WHERE setting_key = $1`, key).Scan(&value)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	return value, nil
}

// UpsertSystemSetting stores or replaces one override.
func UpsertSystemSetting(ctx context.Context, q Querier, key, value string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO system_settings (setting_key, value_json, updated_at)
		VALUES ($1, to_jsonb($2::text), now())
		ON CONFLICT (setting_key) DO UPDATE SET value_json = to_jsonb($2::text), updated_at = now()
	`, key, value)
	return err
}

// SetCdkStatus toggles DISABLED/ACTIVE while refusing terminal transitions.
func SetCdkStatus(ctx context.Context, q Querier, cdkID uuid.UUID, status string) error {
	tag, err := q.Exec(ctx, `
		UPDATE cdks SET status = $2, updated_at = now()
		WHERE id = $1 AND status IN ('UNACTIVATED', 'ACTIVE', 'DISABLED')
	`, cdkID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AdjustCDKQuota applies delta to total and remaining through the ledger
// contract; callers must have already opened the audit transaction.
func AdjustCDKQuota(ctx context.Context, q Querier, cdkID uuid.UUID, delta int64) (QuotaSnapshot, error) {
	tag, err := q.Exec(ctx, `
		UPDATE cdks
		SET quota_total     = quota_total + $2,
		    quota_remaining = quota_remaining + $2,
		    updated_at      = now()
		WHERE id = $1
	`, cdkID, delta)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	if tag.RowsAffected() == 0 {
		return QuotaSnapshot{}, ErrNotFound
	}
	return readQuota(ctx, q, cdkID)
}

// AdminUserRow is the operator-visible user record with its bound CDK.
type AdminUserRow struct {
	ID           uuid.UUID
	Status       string
	CreatedAt    time.Time
	LastLoginAt  *time.Time
	CDKPrefix    *string
	CDKStatus    *string
	CDKRemaining *int64
}

// ListUsers returns platform users newest first.
func ListUsers(ctx context.Context, q Querier, limit int) ([]AdminUserRow, error) {
	rows, err := q.Query(ctx, `
		SELECT u.id, u.status, u.created_at, u.last_login_at,
		       c.code_prefix, c.status, c.quota_remaining
		FROM users u
		LEFT JOIN cdks c ON c.bound_user_id = u.id
		ORDER BY u.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]AdminUserRow, 0, limit)
	for rows.Next() {
		var user AdminUserRow
		if err := rows.Scan(&user.ID, &user.Status, &user.CreatedAt, &user.LastLoginAt,
			&user.CDKPrefix, &user.CDKStatus, &user.CDKRemaining); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// SetUserStatus suspends or restores a user account.
func SetUserStatus(ctx context.Context, q Querier, userID uuid.UUID, status string) error {
	tag, err := q.Exec(ctx, `
		UPDATE users SET status = $2, updated_at = now() WHERE id = $1
	`, userID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminAuditLog is one immutable operator action record.
type AdminAuditLog struct {
	ID          uuid.UUID
	AdminUserID uuid.UUID
	Action      string
	TargetType  string
	TargetID    uuid.UUID
	BeforeJSON  []byte
	AfterJSON   []byte
	Reason      string
	IPMasked    string
	CreatedAt   time.Time
}

// InsertAdminAuditLog appends an audit record; the write path always pairs
// one with a mutation inside the same transaction.
func InsertAdminAuditLog(ctx context.Context, q Querier, entry AdminAuditLog) error {
	_, err := q.Exec(ctx, `
		INSERT INTO admin_audit_logs (id, admin_user_id, action, target_type, target_id,
		                              before_json, after_json, reason, ip_masked)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, entry.ID, entry.AdminUserID, entry.Action, entry.TargetType, entry.TargetID,
		entry.BeforeJSON, entry.AfterJSON, entry.Reason, entry.IPMasked)
	return err
}

// ListAdminAuditLogs returns the newest audit entries.
func ListAdminAuditLogs(ctx context.Context, q Querier, limit int) ([]AdminAuditLog, error) {
	rows, err := q.Query(ctx, `
		SELECT id, admin_user_id, action, target_type, target_id, before_json, after_json,
		       reason, COALESCE(ip_masked, ''), created_at
		FROM admin_audit_logs
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]AdminAuditLog, 0, limit)
	for rows.Next() {
		var entry AdminAuditLog
		if err := rows.Scan(&entry.ID, &entry.AdminUserID, &entry.Action, &entry.TargetType,
			&entry.TargetID, &entry.BeforeJSON, &entry.AfterJSON, &entry.Reason, &entry.IPMasked,
			&entry.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// DashboardCounters aggregates the operator landing metrics.
type DashboardCounters struct {
	UsersTotal       int64
	UsersActive      int64
	CdksActive       int64
	CdksUnactivated  int64
	CdksExhausted    int64
	CallsToday       int64
	CallsFailedToday int64
	QuotaConsumed    int64
}

// GetDashboardCounters reads the admin landing aggregates in one round trip.
func GetDashboardCounters(ctx context.Context, q Querier) (DashboardCounters, error) {
	var counters DashboardCounters
	err := q.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM users WHERE status = 'ACTIVE'),
			(SELECT count(*) FROM cdks WHERE status = 'ACTIVE'),
			(SELECT count(*) FROM cdks WHERE status = 'UNACTIVATED'),
			(SELECT count(*) FROM cdks WHERE status = 'ACTIVE' AND quota_remaining = 0),
			(SELECT count(*) FROM api_calls WHERE accepted_at >= date_trunc('day', now() AT TIME ZONE 'utc')),
			(SELECT count(*) FROM api_calls WHERE accepted_at >= date_trunc('day', now() AT TIME ZONE 'utc') AND status = 'FAILED_REFUNDED'),
			(SELECT COALESCE(sum(quota_used), 0) FROM cdks)
	`).Scan(&counters.UsersTotal, &counters.UsersActive, &counters.CdksActive, &counters.CdksUnactivated,
		&counters.CdksExhausted, &counters.CallsToday, &counters.CallsFailedToday, &counters.QuotaConsumed)
	return counters, err
}
