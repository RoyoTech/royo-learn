package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// SaveProject inserts a new project or returns a ConflictError on duplicate key.
func SaveProject(ctx context.Context, tx *sql.Tx, p *domain.Project) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO projects (id, project_key, display_name, canonical_path, git_remote, fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(p.ID),
		p.ProjectKey,
		p.DisplayName,
		p.CanonicalPath,
		p.GitRemote,
		p.Fingerprint,
		p.CreatedAt.Format(time.RFC3339),
		p.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrDuplicateLearning, "project key already exists: "+p.ProjectKey)
		}
		return fmt.Errorf("SaveProject: %w", err)
	}
	return nil
}

// GetProject retrieves a project by ID.
func GetProject(ctx context.Context, tx *sql.Tx, id domain.ProjectID) (*domain.Project, error) {
	p := &domain.Project{}
	var createdAt, updatedAt string
	err := tx.QueryRowContext(ctx, `
		SELECT id, project_key, display_name, canonical_path, git_remote, fingerprint, created_at, updated_at
		FROM projects WHERE id = ?
	`, string(id)).Scan(
		(*string)(&p.ID),
		&p.ProjectKey,
		&p.DisplayName,
		&p.CanonicalPath,
		&p.GitRemote,
		&p.Fingerprint,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.NewNotFoundError(domain.ErrProjectNotFound, "project")
	}
	if err != nil {
		return nil, fmt.Errorf("GetProject: %w", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return p, nil
}

// GetProjectByKey retrieves a project by its unique project_key.
func GetProjectByKey(ctx context.Context, tx *sql.Tx, key string) (*domain.Project, error) {
	p := &domain.Project{}
	var createdAt, updatedAt string
	err := tx.QueryRowContext(ctx, `
		SELECT id, project_key, display_name, canonical_path, git_remote, fingerprint, created_at, updated_at
		FROM projects WHERE project_key = ?
	`, key).Scan(
		(*string)(&p.ID),
		&p.ProjectKey,
		&p.DisplayName,
		&p.CanonicalPath,
		&p.GitRemote,
		&p.Fingerprint,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.NewNotFoundError(domain.ErrProjectNotFound, fmt.Sprintf("project with key %q", key))
	}
	if err != nil {
		return nil, fmt.Errorf("GetProjectByKey: %w", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return p, nil
}

// isUniqueViolation checks if the error is a SQLite UNIQUE constraint violation.
func isUniqueViolation(err error) bool {
	return err != nil && contains(err.Error(), "UNIQUE constraint failed")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// marshalStringSlice marshals a string slice to JSON.
func marshalStringSlice(v []string) string {
	if len(v) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// marshalAny marshals any value to JSON, returning "{}" on failure.
func marshalAny(v interface{}) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
