// Package domain declares the platform entities and their status vocabulary.
// The database schema in migrations keeps these columns authoritative.
package domain

import (
	"time"

	"github.com/google/uuid"
)

type CDKStatus string

const (
	CDKStatusUnactivated CDKStatus = "UNACTIVATED"
	CDKStatusActive      CDKStatus = "ACTIVE"
	CDKStatusDisabled    CDKStatus = "DISABLED"
	CDKStatusExpired     CDKStatus = "EXPIRED"
)

type APIKeyStatus string

const (
	APIKeyStatusActive   APIKeyStatus = "ACTIVE"
	APIKeyStatusDisabled APIKeyStatus = "DISABLED"
	APIKeyStatusDeleted  APIKeyStatus = "DELETED"
)

type UserStatus string

const (
	UserStatusActive    UserStatus = "ACTIVE"
	UserStatusSuspended UserStatus = "SUSPENDED"
)

type User struct {
	ID          uuid.UUID
	Status      UserStatus
	LastLoginAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CdkBatch struct {
	ID                  uuid.UUID
	Name                string
	Description         *string
	DefaultQuota        int64
	ActivationDeadline  *time.Time
	ServiceDurationDays *int
	CreatedBy           *uuid.UUID
	CreatedAt           time.Time
}

type Cdk struct {
	ID                 uuid.UUID
	BatchID            uuid.UUID
	CodePrefix         string
	CodeHash           []byte
	CodeCiphertext     []byte
	Remark             *string
	Status             CDKStatus
	BoundUserID        *uuid.UUID
	ActivationDeadline *time.Time
	ExpiresAt          *time.Time
	QuotaTotal         int64
	QuotaUsed          int64
	QuotaReserved      int64
	QuotaRemaining     int64
	ActivatedAt        *time.Time
	LastUsedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type APIKey struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	KeyPrefix  string
	KeyLast4   string
	KeyHash    []byte
	Status     APIKeyStatus
	QuotaLimit *int64
	AllowedIPs *string
	TotalCalls int64
	LastUsedAt *time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

type UserSession struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash []byte
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}
