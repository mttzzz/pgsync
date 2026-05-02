package screens

import (
	"fmt"
	"strconv"

	"github.com/mttzzz/pgsync/internal/config"
)

// FieldSpec describes a ConfigEditor field without depending on huh internals.
type FieldSpec struct {
	Key         string
	Label       string
	Description string
	Value       string
	Secret      bool
	Validate    func(string) error
}

// RemoteFields returns remote connection fields.
func RemoteFields(cfg config.Config) []FieldSpec {
	return []FieldSpec{
		{Key: "remote.host", Label: "Хост prod", Description: "Адрес удалённого PostgreSQL", Value: cfg.Remote.Host, Validate: config.ValidateHost},
		{Key: "remote.port", Label: "Порт prod", Description: "Порт удалённого PostgreSQL", Value: strconv.Itoa(cfg.Remote.Port), Validate: validatePortString},
		{Key: "remote.user", Label: "Пользователь prod", Description: "Пользователь для чтения", Value: cfg.Remote.User, Validate: requiredString},
		{Key: "remote.password", Label: "Пароль prod", Description: "Пароль удалённой БД", Value: cfg.Remote.Password, Secret: true, Validate: nil},
		{Key: "remote.database", Label: "База по умолчанию", Description: "База для синхронизации по умолчанию", Value: cfg.Remote.Database, Validate: nil},
		{Key: "remote.ssl_mode", Label: "SSL mode prod", Description: "disable/require/verify-ca/verify-full", Value: cfg.Remote.SSLMode, Validate: config.ValidateSSLMode},
		{Key: "remote.proxy_url", Label: "Прокси", Description: "SOCKS5/HTTP proxy URL", Value: cfg.Remote.ProxyURL, Validate: config.ValidateProxyURL},
	}
}

// LocalFields returns local connection fields.
func LocalFields(cfg config.Config) []FieldSpec {
	return []FieldSpec{
		{Key: "local.host", Label: "Хост local", Description: "Адрес локального PostgreSQL", Value: cfg.Local.Host, Validate: config.ValidateHost},
		{Key: "local.port", Label: "Порт local", Description: "Порт локального PostgreSQL", Value: strconv.Itoa(cfg.Local.Port), Validate: validatePortString},
		{Key: "local.user", Label: "Пользователь local", Description: "Пользователь локальной БД", Value: cfg.Local.User, Validate: requiredString},
		{Key: "local.password", Label: "Пароль local", Description: "Пароль локальной БД", Value: cfg.Local.Password, Secret: true, Validate: nil},
		{Key: "local.ssl_mode", Label: "SSL mode local", Description: "disable/require/verify-ca/verify-full", Value: cfg.Local.SSLMode, Validate: config.ValidateSSLMode},
	}
}

// RuntimeFields returns runtime fields.
func RuntimeFields(cfg config.Config) []FieldSpec {
	return []FieldSpec{
		{Key: "runtime.threads", Label: "Потоки", Description: "Количество потоков COPY", Value: strconv.Itoa(cfg.Runtime.Threads), Validate: validatePositiveInt},
		{Key: "runtime.engine", Label: "Движок", Description: "auto/native/external", Value: cfg.Runtime.Engine, Validate: validateEngineString},
		{Key: "runtime.default_database", Label: "База по умолчанию", Description: "Используется если команда без db", Value: cfg.Runtime.DefaultDatabase, Validate: nil},
	}
}

// LoggingFields returns logging fields.
func LoggingFields(cfg config.Config) []FieldSpec {
	return []FieldSpec{
		{Key: "logging.level", Label: "Уровень логов", Description: "debug/info/warn/error", Value: cfg.Logging.Level, Validate: validateLogLevel},
		{Key: "logging.format", Label: "Формат логов", Description: "text/json", Value: cfg.Logging.Format, Validate: validateLogFormat},
	}
}

func requiredString(value string) error {
	if value == "" {
		return fmt.Errorf("обязательное поле")
	}
	return nil
}

func validatePortString(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	return config.ValidatePort(port)
}

func validatePositiveInt(value string) error {
	val, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	if val < 1 {
		return fmt.Errorf("значение должно быть >= 1")
	}
	return nil
}

func validateEngineString(value string) error {
	switch value {
	case "auto", "native", "external":
		return nil
	default:
		return fmt.Errorf("движок должен быть auto/native/external")
	}
}

func validateLogLevel(value string) error {
	switch value {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("уровень должен быть debug/info/warn/error")
	}
}

func validateLogFormat(value string) error {
	switch value {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("формат должен быть text/json")
	}
}
