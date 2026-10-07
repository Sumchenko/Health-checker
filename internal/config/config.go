package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DB DB

	// CheckInterval — интервал проверок по умолчанию для целей без собственного интервала.
	CheckInterval time.Duration
	// CheckTimeout — таймаут одной проверки по умолчанию.
	CheckTimeout time.Duration
	// ReloadInterval — как часто перечитывать список целей из БД.
	ReloadInterval time.Duration
	// Workers — максимум одновременно выполняющихся проверок.
	Workers int
}

type DB struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// DSN возвращает строку подключения в формате URL с экранированием логина и пароля.
func (d DB) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(d.User, d.Password),
		Host:     d.Host + ":" + d.Port,
		Path:     d.Name,
		RawQuery: url.Values{"sslmode": {d.SSLMode}}.Encode(),
	}
	return u.String()
}

func Load() (Config, error) {
	cfg := Config{
		DB: DB{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "password"),
			Name:     getEnv("DB_NAME", "health_db"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
	}

	var err error
	if cfg.CheckInterval, err = getDuration("CHECK_INTERVAL", time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.CheckTimeout, err = getDuration("CHECK_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ReloadInterval, err = getDuration("TARGETS_RELOAD_INTERVAL", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.Workers, err = getInt("WORKERS", 5); err != nil {
		return Config{}, err
	}

	if cfg.CheckInterval <= 0 || cfg.CheckTimeout <= 0 || cfg.ReloadInterval <= 0 || cfg.Workers <= 0 {
		return Config{}, fmt.Errorf("CHECK_INTERVAL, CHECK_TIMEOUT, TARGETS_RELOAD_INTERVAL и WORKERS должны быть положительными")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("некорректное значение %s=%q: %w", key, value, err)
	}
	return d, nil
}

func getInt(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("некорректное значение %s=%q: %w", key, value, err)
	}
	return n, nil
}
