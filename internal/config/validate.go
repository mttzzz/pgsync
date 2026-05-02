package config

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

var validSSLModes = []string{"disable", "require", "verify-ca", "verify-full"}
var validProxySchemes = []string{"socks5", "socks5h", "http", "https"}

/* ValidateHost validates a PostgreSQL host field. */
func ValidateHost(host string) error {
	if host == "" {
		return errors.New("host is required")
	}
	if strings.ContainsAny(host, " \t\n") {
		return errors.New("host must not contain whitespace")
	}
	return nil
}

/* ValidatePort validates a TCP port. */
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port out of range: %d", port)
	}
	return nil
}

/* ValidateSSLMode validates a libpq-compatible sslmode value. */
func ValidateSSLMode(mode string) error {
	if !slices.Contains(validSSLModes, mode) {
		return fmt.Errorf("ssl_mode must be one of %v, got %q", validSSLModes, mode)
	}
	return nil
}

/* ValidateProxyURL validates an optional proxy URL. */
func ValidateProxyURL(raw string) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse proxy url: %w", err)
	}
	if !slices.Contains(validProxySchemes, parsed.Scheme) {
		return fmt.Errorf("proxy scheme must be one of %v, got %q", validProxySchemes, parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("proxy url has no host")
	}
	return nil
}

/* Validate returns the first error found, or nil if cfg is fully valid. */
func Validate(cfg Config) error {
	checks := []func() error{
		func() error { return ValidateHost(cfg.Remote.Host) },
		func() error { return ValidatePort(cfg.Remote.Port) },
		func() error { return ValidateSSLMode(cfg.Remote.SSLMode) },
		func() error { return ValidateProxyURL(cfg.Remote.ProxyURL) },
		func() error { return ValidateHost(cfg.Local.Host) },
		func() error { return ValidatePort(cfg.Local.Port) },
		func() error { return ValidateSSLMode(cfg.Local.SSLMode) },
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	if cfg.Runtime.Threads < 1 {
		return errors.New("runtime.threads must be >= 1")
	}
	if !slices.Contains([]string{"native", "external", "auto"}, cfg.Runtime.Engine) {
		return errors.New("runtime.engine must be native|external|auto")
	}
	if !slices.Contains([]string{"debug", "info", "warn", "error"}, cfg.Logging.Level) {
		return errors.New("logging.level must be debug|info|warn|error")
	}
	if !slices.Contains([]string{"text", "json"}, cfg.Logging.Format) {
		return errors.New("logging.format must be text|json")
	}
	return nil
}
