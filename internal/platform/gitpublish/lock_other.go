//go:build !linux && !darwin

package gitpublish

import (
	"context"
	"os"
)

type repositoryLock struct {
	file *os.File
}

func acquireLock(ctx context.Context, path string) (*repositoryLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return &repositoryLock{file: file}, nil
}

func (l *repositoryLock) release() {
	if l != nil && l.file != nil {
		_ = l.file.Close()
	}
}
