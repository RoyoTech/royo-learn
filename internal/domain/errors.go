package domain

import (
	"errors"
	"fmt"
)

// ErrorCode is a stable machine-readable error identifier.
type ErrorCode string

const (
	ErrInvalidArgument       ErrorCode = "invalid_argument"
	ErrInvalidConfig         ErrorCode = "invalid_config"
	ErrProjectNotFound       ErrorCode = "project_not_found"
	ErrAmbiguousProject      ErrorCode = "ambiguous_project"
	ErrUnknownProject        ErrorCode = "unknown_project"
	ErrLearningNotFound      ErrorCode = "learning_not_found"
	ErrInvalidTransition     ErrorCode = "invalid_transition"
	ErrDuplicateLearning     ErrorCode = "duplicate_learning"
	ErrEvidenceMissing       ErrorCode = "evidence_missing"
	ErrEvidenceTooLarge      ErrorCode = "evidence_too_large"
	ErrSecretDetected        ErrorCode = "secret_detected"
	ErrPathOutsideRoot       ErrorCode = "path_outside_root"
	ErrSymlinkEscape         ErrorCode = "symlink_escape"
	ErrProtectedPath         ErrorCode = "protected_path"
	ErrTargetAmbiguous       ErrorCode = "target_ambiguous"
	ErrTargetChanged         ErrorCode = "target_changed"
	ErrDirtyTarget           ErrorCode = "dirty_target"
	ErrApprovalRequired      ErrorCode = "approval_required"
	ErrApprovalInvalid       ErrorCode = "approval_invalid"
	ErrApprovalExpired       ErrorCode = "approval_expired"
	ErrPreviewNotFound       ErrorCode = "preview_not_found"
	ErrPreviewHashMismatch   ErrorCode = "preview_hash_mismatch"
	ErrPublicationConflict   ErrorCode = "publication_conflict"
	ErrVerificationFailed    ErrorCode = "verification_failed"
	ErrRollbackConflict      ErrorCode = "rollback_conflict"
	ErrRollbackFailed        ErrorCode = "rollback_failed"
	ErrPublicationFailed     ErrorCode = "publication_failed"
	ErrDatabaseLocked        ErrorCode = "database_locked"
	ErrDatabaseCorrupt       ErrorCode = "database_corrupt"
	ErrMigrationChecksum     ErrorCode = "migration_checksum_mismatch"
	ErrRecordHashMismatch    ErrorCode = "record_hash_mismatch"
	ErrEngramUnavailable     ErrorCode = "engram_unavailable"
	ErrEngramAmbiguous       ErrorCode = "engram_ambiguous_project"
	ErrGentleAIUnavailable   ErrorCode = "gentle_ai_unavailable"
	ErrSkillRegistryFailed   ErrorCode = "skill_registry_failed"
	ErrMCPProtocolError      ErrorCode = "mcp_protocol_error"
	ErrPayloadTooLarge       ErrorCode = "payload_too_large"
	ErrExternalCommandFailed ErrorCode = "external_command_failed"
	ErrTimeout               ErrorCode = "timeout"
)

// AllErrorCodes returns every stable error code the domain models. It is the
// authoritative list the exit-code mapping and the docs/17 catalog are checked
// against.
func AllErrorCodes() []ErrorCode {
	return []ErrorCode{
		ErrInvalidArgument, ErrInvalidConfig, ErrProjectNotFound, ErrAmbiguousProject,
		ErrUnknownProject, ErrLearningNotFound, ErrInvalidTransition, ErrDuplicateLearning,
		ErrEvidenceMissing, ErrEvidenceTooLarge, ErrSecretDetected, ErrPathOutsideRoot,
		ErrSymlinkEscape, ErrProtectedPath, ErrTargetAmbiguous, ErrTargetChanged,
		ErrDirtyTarget, ErrApprovalRequired, ErrApprovalInvalid, ErrApprovalExpired,
		ErrPreviewNotFound, ErrPreviewHashMismatch, ErrPublicationConflict, ErrVerificationFailed,
		ErrRollbackConflict, ErrRollbackFailed, ErrPublicationFailed, ErrDatabaseLocked,
		ErrDatabaseCorrupt, ErrMigrationChecksum, ErrRecordHashMismatch, ErrEngramUnavailable,
		ErrEngramAmbiguous, ErrGentleAIUnavailable, ErrSkillRegistryFailed, ErrMCPProtocolError,
		ErrPayloadTooLarge, ErrExternalCommandFailed, ErrTimeout,
	}
}

// ExitCode maps a stable error code to the CLI exit code documented in
// docs/04-CLI-SPEC.md §Exit codes. It is the single source both the CLI and the
// MCP handlers derive their exit codes from; no surface picks a constant by hand
// or interprets an error by string matching. An unmodeled code falls through to
// the generic failure (1).
func (c ErrorCode) ExitCode() int {
	switch c {
	case ErrInvalidArgument, ErrEvidenceMissing, ErrEvidenceTooLarge, ErrPayloadTooLarge:
		return 2
	case ErrInvalidConfig:
		return 3
	case ErrProjectNotFound, ErrAmbiguousProject, ErrUnknownProject:
		return 4
	case ErrLearningNotFound, ErrPreviewNotFound:
		return 5
	case ErrInvalidTransition:
		return 6
	case ErrApprovalRequired, ErrApprovalInvalid, ErrApprovalExpired:
		return 7
	case ErrDuplicateLearning, ErrTargetAmbiguous, ErrTargetChanged, ErrDirtyTarget,
		ErrPublicationConflict, ErrPreviewHashMismatch, ErrRollbackConflict:
		return 8
	case ErrVerificationFailed:
		return 9
	case ErrEngramUnavailable, ErrEngramAmbiguous, ErrGentleAIUnavailable, ErrSkillRegistryFailed:
		return 10
	case ErrPathOutsideRoot, ErrSymlinkEscape, ErrProtectedPath, ErrSecretDetected:
		return 11
	case ErrDatabaseCorrupt, ErrMigrationChecksum, ErrRecordHashMismatch:
		return 12
	case ErrDatabaseLocked, ErrRollbackFailed, ErrPublicationFailed:
		return 13
	case ErrMCPProtocolError:
		return 14
	case ErrExternalCommandFailed, ErrTimeout:
		return 15
	default:
		return 1
	}
}

// AsDomainError extracts the underlying *DomainError from err, unwrapping typed
// wrappers (NotFoundError, ConflictError, ValidationError, PermissionError) and
// any error chain. It reports false when err carries no domain error, so callers
// translate by code, never by string matching.
func AsDomainError(err error) (*DomainError, bool) {
	if err == nil {
		return nil, false
	}
	// The typed wrappers embed *DomainError; match them first, because
	// errors.As targeting *DomainError would skip past the wrapper to its Cause.
	var (
		nf *NotFoundError
		cf *ConflictError
		ve *ValidationError
		pf *PermissionError
	)
	switch {
	case errors.As(err, &nf):
		return nf.DomainError, true
	case errors.As(err, &cf):
		return cf.DomainError, true
	case errors.As(err, &ve):
		return ve.DomainError, true
	case errors.As(err, &pf):
		return pf.DomainError, true
	}
	var de *DomainError
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}

// DomainError is a typed error with a stable code and human-readable message.
type DomainError struct {
	Code        ErrorCode
	Message     string
	Recoverable bool
	Details     map[string]any
	NextAction  string
	Cause       error
}

func (e *DomainError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Cause
}

// NotFoundError signals that a requested entity was not found.
type NotFoundError struct{ *DomainError }

// ConflictError signals a conflict (e.g., duplicate key, dirty state).
type ConflictError struct{ *DomainError }

// ValidationError signals that input failed validation rules.
type ValidationError struct{ *DomainError }

// PermissionError signals that the actor lacks permission.
type PermissionError struct{ *DomainError }

// --- Constructors ----------------------------------------------

func NewNotFoundError(code ErrorCode, entity string) *NotFoundError {
	return &NotFoundError{
		&DomainError{
			Code:        code,
			Message:     fmt.Sprintf("%s not found", entity),
			Recoverable: true,
			NextAction:  "verify the identifier and try again",
		},
	}
}

func NewConflictError(code ErrorCode, msg string) *ConflictError {
	return &ConflictError{
		&DomainError{
			Code:        code,
			Message:     msg,
			Recoverable: false,
			NextAction:  "resolve the conflict before retrying",
		},
	}
}

func NewValidationError(code ErrorCode, msg string) *ValidationError {
	return &ValidationError{
		&DomainError{
			Code:        code,
			Message:     msg,
			Recoverable: true,
			NextAction:  "fix the input and try again",
		},
	}
}

func NewPermissionError(code ErrorCode, msg string) *PermissionError {
	return &PermissionError{
		&DomainError{
			Code:        code,
			Message:     msg,
			Recoverable: false,
			NextAction:  "obtain the required permission or change scope",
		},
	}
}
