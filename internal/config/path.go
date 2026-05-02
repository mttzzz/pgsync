package config

import (
	"errors"
	"path/filepath"
	"runtime"
)

const (
	appDir   = "pgsync"
	fileName = "config.toml"
)

/* DefaultPath resolves the config file path from environment variables.
 * Pass a map, typically os.Environ-derived, to keep tests process-local.
 */
func DefaultPath(env map[string]string) (string, error) {
	return defaultPath(runtime.GOOS, env)
}

func defaultPath(goos string, env map[string]string) (string, error) {
	if goos == "windows" {
		appData := env["APPDATA"]
		if appData == "" {
			return "", errors.New("APPDATA not set")
		}
		return filepath.Join(appData, appDir, fileName), nil
	}
	if xdg := env["XDG_CONFIG_HOME"]; xdg != "" {
		return filepath.Join(xdg, appDir, fileName), nil
	}
	home := env["HOME"]
	if home == "" {
		return "", errors.New("HOME not set")
	}
	return filepath.Join(home, ".config", appDir, fileName), nil
}
