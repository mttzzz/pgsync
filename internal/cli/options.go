package cli

import (
	"errors"
	"fmt"
	"slices"
)

// Options contains global presentation/runtime options after flag parsing.
type Options struct {
	Output           string
	Quiet            bool
	Verbose          bool
	NoColor          bool
	ConfigPath       string
	Threads          int
	Engine           string
	UseSystemPgtools bool
}

// Validate validates global CLI options.
func (o Options) Validate() error {
	if err := ValidateOutputMode(o.Output); err != nil {
		return err
	}
	if o.Engine != "" {
		return ValidateEngineMode(o.Engine)
	}
	if o.Threads < 0 {
		return fmt.Errorf("threads must be >= 0")
	}
	return nil
}

// ValidateOutputMode validates the global --output value.
func ValidateOutputMode(mode string) error {
	if !slices.Contains([]string{"text", "json"}, mode) {
		return fmt.Errorf("output must be text|json")
	}
	return nil
}

// ValidateEngineMode validates the global --engine value.
func ValidateEngineMode(mode string) error {
	if !slices.Contains([]string{"auto", "native", "external"}, mode) {
		return fmt.Errorf("engine must be auto|native|external")
	}
	return nil
}

// LogLevel maps quiet/verbose flags to slog/config log levels.
func (o Options) LogLevel(defaultLevel string) string {
	if o.Quiet {
		return "error"
	}
	if o.Verbose {
		return "debug"
	}
	if defaultLevel == "" {
		return "info"
	}
	return defaultLevel
}

// ErrInvalidOptions marks invalid global option combinations.
var ErrInvalidOptions = errors.New("invalid options")
