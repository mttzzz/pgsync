package config

import "net/url"

const redactedSecret = "xxxxx"

// Redacted returns a copy of cfg with passwords and proxy credentials masked.
func Redacted(cfg Config) Config {
	cfg.Remote.Password = redactValue(cfg.Remote.Password)
	cfg.Local.Password = redactValue(cfg.Local.Password)
	cfg.Remote.ProxyURL = RedactProxyURL(cfg.Remote.ProxyURL)
	return cfg
}

// RedactProxyURL masks the password part of a proxy URL, if present.
func RedactProxyURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return raw
	}
	if _, ok := parsed.User.Password(); !ok {
		return raw
	}
	parsed.User = url.UserPassword(parsed.User.Username(), redactedSecret)
	return parsed.String()
}

func redactValue(value string) string {
	if value == "" {
		return ""
	}
	return redactedSecret
}
