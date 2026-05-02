package native

import (
	"errors"
	"log/slog"

	"github.com/mttzzz/pgsync/internal/engine"
)

const phase2ExternalEngineMessage = "external engine not implemented in Phase 2"

func preparePlanOptions(opts *engine.PlanOptions, logger *slog.Logger) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if err := selectPhase2Mode(opts); err != nil {
		return err
	}
	warnSystemPgtools(opts.UseSystemPgtools, logger)
	return nil
}

func selectPhase2Mode(opts *engine.PlanOptions) error {
	switch opts.Mode {
	case engine.ModeAuto:
		opts.Mode = engine.ModeNative
		return nil
	case engine.ModeNative:
		return nil
	case engine.ModeExternal:
		return errors.New(phase2ExternalEngineMessage)
	default:
		return nil
	}
}

func warnSystemPgtools(useSystem bool, logger *slog.Logger) {
	if useSystem || logger == nil {
		return
	}
	logger.Warn("embedded PostgreSQL tools are not available in Phase 2; using system pg_dump/pg_restore")
}
