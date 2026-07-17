package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agent-royo-learn/internal/domain"

	"github.com/google/uuid"
)

const publicationLockPath = ".royo-learn/publication.lock"

type publicationLockOwner struct {
	Token     string    `json:"token"`
	Owner     string    `json:"owner"`
	Operation string    `json:"operation"`
	PID       int       `json:"pid"`
	Acquired  time.Time `json:"acquired_at"`
}

type publicationLock struct {
	root  *os.Root
	owner publicationLockOwner
}

func acquirePublicationLock(projectRoot, operation string, actor domain.Actor) (*publicationLock, error) {
	if _, err := secureRelativePath(projectRoot, publicationLockPath, "publication lock", true); err != nil {
		return nil, err
	}
	root, err := openRootNoFollow(projectRoot)
	if err != nil {
		return nil, err
	}
	ownerName := actor.Name
	if ownerName == "" {
		ownerName = actor.Kind
	}
	owner := publicationLockOwner{
		Token: uuid.NewString(), Owner: ownerName, Operation: operation,
		PID: os.Getpid(), Acquired: utcNowPublish(),
	}
	data, _ := json.Marshal(owner)
	f, err := root.OpenFile(filepath.FromSlash(publicationLockPath), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			existing, readErr := root.ReadFile(filepath.FromSlash(publicationLockPath))
			var current publicationLockOwner
			if readErr == nil {
				_ = json.Unmarshal(existing, &current)
			}
			_ = root.Close()
			message := "publication lock is held"
			if current.Owner != "" {
				message += fmt.Sprintf(" by %s for %s", current.Owner, current.Operation)
			}
			return nil, &domain.ConflictError{DomainError: &domain.DomainError{
				Code: domain.ErrPublicationConflict, Message: message, Recoverable: true,
				Details: map[string]any{
					"owner": current.Owner, "operation": current.Operation, "pid": current.PID,
					"acquired_at": current.Acquired, "stale": !current.Acquired.IsZero() && time.Since(current.Acquired) > 30*time.Minute,
				},
				NextAction: "wait for the owner to finish; if it crashed, inspect in-progress publications before removing .royo-learn/publication.lock",
			}}
		}
		_ = root.Close()
		return nil, fmt.Errorf("acquire publication lock: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		_ = root.Remove(filepath.FromSlash(publicationLockPath))
		_ = root.Close()
		return nil, fmt.Errorf("write publication lock: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = root.Remove(filepath.FromSlash(publicationLockPath))
		_ = root.Close()
		return nil, fmt.Errorf("sync publication lock: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = root.Remove(filepath.FromSlash(publicationLockPath))
		_ = root.Close()
		return nil, fmt.Errorf("close publication lock: %w", err)
	}
	return &publicationLock{root: root, owner: owner}, nil
}

func (l *publicationLock) Release() error {
	if l == nil || l.root == nil {
		return nil
	}
	defer l.root.Close()
	data, err := l.root.ReadFile(filepath.FromSlash(publicationLockPath))
	if err != nil {
		return fmt.Errorf("read publication lock for release: %w", err)
	}
	var current publicationLockOwner
	if err := json.Unmarshal(data, &current); err != nil || current.Token != l.owner.Token {
		return fmt.Errorf("publication lock ownership changed before release")
	}
	if err := l.root.Remove(filepath.FromSlash(publicationLockPath)); err != nil {
		return fmt.Errorf("remove publication lock: %w", err)
	}
	l.root = nil
	return nil
}
