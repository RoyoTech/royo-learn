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

	// Experience-discovery ingestion codes (Hito 1). See
	// docs/22-ADAPTER-CONTRACT.md and docs/17-ERROR-CODES.md.
	ErrExperienceSourceNotFound     ErrorCode = "experience_source_not_found"
	ErrExperienceSchemaUnsupported  ErrorCode = "experience_source_schema_unsupported"
	ErrExperienceTurnIncomplete     ErrorCode = "experience_turn_incomplete"
	ErrExperienceLocatorInvalid     ErrorCode = "experience_locator_invalid"
	ErrExperienceLocatorOutsideRoot ErrorCode = "experience_locator_outside_root"
	ErrExperiencePayloadTooLarge    ErrorCode = "experience_payload_too_large"
	ErrExperienceRevisionConflict   ErrorCode = "experience_revision_conflict"
	ErrExperienceCursorConflict     ErrorCode = "experience_cursor_conflict"
)

// AllErrorCodes returns every stable error code modeled by the domain.
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
		ErrExperienceSourceNotFound, ErrExperienceSchemaUnsupported,
		ErrExperienceTurnIncomplete, ErrExperienceLocatorInvalid,
		ErrExperienceLocatorOutsideRoot, ErrExperiencePayloadTooLarge,
		ErrExperienceRevisionConflict, ErrExperienceCursorConflict,
	}
}

// ExitCode maps a stable error code to docs/04-CLI-SPEC.md exit codes.
func (c ErrorCode) ExitCode() int {
	switch c {
	case ErrInvalidArgument, ErrEvidenceMissing, ErrEvidenceTooLarge, ErrPayloadTooLarge,
		ErrExperienceTurnIncomplete, ErrExperienceLocatorInvalid, ErrExperiencePayloadTooLarge:
		return 2
	case ErrInvalidConfig, ErrExperienceSchemaUnsupported:
		return 3
	case ErrProjectNotFound, ErrAmbiguousProject, ErrUnknownProject:
		return 4
	case ErrLearningNotFound, ErrPreviewNotFound, ErrExperienceSourceNotFound:
		return 5
	case ErrInvalidTransition:
		return 6
	case ErrApprovalRequired, ErrApprovalInvalid, ErrApprovalExpired:
		return 7
	case ErrDuplicateLearning, ErrTargetAmbiguous, ErrTargetChanged, ErrDirtyTarget,
		ErrPublicationConflict, ErrPreviewHashMismatch, ErrRollbackConflict,
		ErrExperienceRevisionConflict, ErrExperienceCursorConflict:
		return 8
	case ErrVerificationFailed:
		return 9
	case ErrEngramUnavailable, ErrEngramAmbiguous, ErrGentleAIUnavailable, ErrSkillRegistryFailed:
		return 10
	case ErrPathOutsideRoot, ErrSymlinkEscape, ErrProtectedPath, ErrSecretDetected,
		ErrExperienceLocatorOutsideRoot:
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

// AsDomainError extracts a DomainError from typed wrappers and error chains.
func AsDomainError(err error) (*DomainError, bool) {
	if err == nil {
		return nil, false
	}
	var (
		notFound   *NotFoundError
		conflict   *ConflictError
		validation *ValidationError
		permission *PermissionError
	)
	switch {
	case errors.As(err, &notFound):
		return notFound.DomainError, true
	case errors.As(err, &conflict):
		return conflict.DomainError, true
	case errors.As(err, &validation):
		return validation.DomainError, true
	case errors.As(err, &permission):
		return permission.DomainError, true
	}
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return domainErr, true
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
