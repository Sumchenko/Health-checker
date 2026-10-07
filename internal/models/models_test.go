package models

import (
	"strings"
	"testing"
	"time"
)

func TestTargetValidate(t *testing.T) {
	valid := Target{Name: "Портфолио", URL: "https://example.com/health", Interval: time.Minute, Timeout: 10 * time.Second}

	tests := []struct {
		name    string
		modify  func(*Target)
		wantErr string
	}{
		{name: "корректная цель", modify: func(*Target) {}},
		{name: "значения по умолчанию", modify: func(t *Target) { t.Interval, t.Timeout = 0, 0 }},
		{name: "пустое название", modify: func(t *Target) { t.Name = "  " }, wantErr: "название"},
		{name: "нет схемы", modify: func(t *Target) { t.URL = "example.com" }, wantErr: "URL"},
		{name: "чужая схема", modify: func(t *Target) { t.URL = "ftp://example.com" }, wantErr: "URL"},
		{name: "код вне диапазона", modify: func(t *Target) { t.ExpectedStatus = 42 }, wantErr: "HTTP-код"},
		{name: "таймаут больше интервала", modify: func(t *Target) { t.Timeout = 2 * time.Minute }, wantErr: "таймаут"},
		{name: "отрицательный интервал", modify: func(t *Target) { t.Interval = -time.Second }, wantErr: "отрицательными"},
		{name: "пустой заголовок", modify: func(t *Target) { t.Headers = map[string]string{"": "x"} }, wantErr: "заголовка"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := valid
			tt.modify(&target)
			err := target.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("неожиданная ошибка: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ошибка %v, want содержит %q", err, tt.wantErr)
			}
		})
	}
}

func TestTargetEqual(t *testing.T) {
	a := Target{ID: 1, Name: "a", URL: "https://a.test", Headers: map[string]string{"apikey": "1"}}
	b := a
	b.Headers = map[string]string{"apikey": "1"}
	if !a.Equal(b) {
		t.Error("одинаковые цели должны быть равны")
	}

	b.Headers = map[string]string{"apikey": "2"}
	if a.Equal(b) {
		t.Error("цели с разными заголовками не должны быть равны")
	}

	c := a
	c.Keyword = "Привет"
	if a.Equal(c) {
		t.Error("цели с разными ключевыми словами не должны быть равны")
	}
}
