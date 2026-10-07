package config

import (
	"testing"
	"time"
)

func TestDSN_EscapesCredentials(t *testing.T) {
	db := DB{Host: "db", Port: "5432", User: "user", Password: "p@ss:w/rd", Name: "health_db", SSLMode: "disable"}

	want := "postgres://user:p%40ss%3Aw%2Frd@db:5432/health_db?sslmode=disable"
	if got := db.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestLoad(t *testing.T) {
	t.Setenv("CHECK_INTERVAL", "2m")
	t.Setenv("CHECK_TIMEOUT", "")
	t.Setenv("WORKERS", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CheckInterval != 2*time.Minute || cfg.CheckTimeout != 15*time.Second || cfg.Workers != 3 {
		t.Errorf("неожиданный конфиг: %+v", cfg)
	}
}

func TestLoad_Invalid(t *testing.T) {
	for _, env := range []struct{ key, value string }{
		{"CHECK_INTERVAL", "минута"},
		{"CHECK_TIMEOUT", "-1s"},
		{"WORKERS", "0"},
	} {
		t.Run(env.key, func(t *testing.T) {
			t.Setenv(env.key, env.value)
			if _, err := Load(); err == nil {
				t.Errorf("ожидалась ошибка для %s=%q", env.key, env.value)
			}
		})
	}
}
