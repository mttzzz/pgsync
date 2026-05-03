package pgtools

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/mttzzz/pgsync/internal/runner"
)

// Signer applies platform-specific signing to extracted pgtools executables.
type Signer interface {
	ShouldSign(platform, name string) bool
	Sign(ctx context.Context, path string) error
}

type noopSigner struct{}

func (noopSigner) ShouldSign(string, string) bool     { return false }
func (noopSigner) Sign(context.Context, string) error { return nil }

type darwinSigner struct {
	runner runner.CommandRunner
	logger *slog.Logger
}

// NewSigner returns the platform signer for goos.
func NewSigner(goos string, commandRunner runner.CommandRunner, logger *slog.Logger) Signer {
	if goos != "darwin" {
		return noopSigner{}
	}
	return darwinSigner{runner: commandRunner, logger: logger}
}

func (s darwinSigner) ShouldSign(platform, name string) bool {
	if !strings.HasPrefix(platform, "darwin-") {
		return false
	}
	base := filepath.Base(name)
	return base == "pg_dump" || base == "pg_restore"
}

func (s darwinSigner) Sign(ctx context.Context, path string) error {
	if s.runner == nil {
		return fmt.Errorf("codesign runner is required")
	}
	_, stderr, err := s.runner.Run(ctx, "codesign", []string{"--sign", "-", "--identifier", "dev.pgsync.bundled", path}, nil)
	if err != nil {
		message := strings.TrimSpace(string(stderr))
		if strings.Contains(message, "is already signed") {
			return nil
		}
		return fmt.Errorf("codesign %s failed: %w: %s", path, err, message)
	}
	if s.logger != nil {
		s.logger.Debug("signed embedded pgtools binary", "path", path)
	}
	return nil
}
