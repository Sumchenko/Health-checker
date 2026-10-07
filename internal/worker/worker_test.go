package worker

import (
	"context"
	"health-checker/internal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		timeout   time.Duration
		wantUp    bool
		wantCode  int
		wantError string
	}{
		{
			name:     "200 — цель доступна",
			handler:  func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) },
			wantUp:   true,
			wantCode: http.StatusOK,
		},
		{
			name:      "500 — цель недоступна",
			handler:   func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantCode:  http.StatusInternalServerError,
			wantError: "неожиданный HTTP-код 500",
		},
		{
			name:      "404 — цель недоступна",
			handler:   http.NotFound,
			wantCode:  http.StatusNotFound,
			wantError: "неожиданный HTTP-код 404",
		},
		{
			name: "медленный ответ — таймаут",
			handler: func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-time.After(time.Second):
				case <-r.Context().Done():
				}
			},
			timeout:   50 * time.Millisecond,
			wantError: "таймаут",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			w := NewWorker(time.Second)
			res := w.Check(context.Background(), models.Target{ID: 7, URL: srv.URL, Timeout: tt.timeout})

			if res.IsUp != tt.wantUp {
				t.Errorf("IsUp = %v, want %v (err: %q)", res.IsUp, tt.wantUp, res.Error)
			}
			if res.StatusCode != tt.wantCode {
				t.Errorf("StatusCode = %d, want %d", res.StatusCode, tt.wantCode)
			}
			if !strings.Contains(res.Error, tt.wantError) {
				t.Errorf("Error = %q, want contains %q", res.Error, tt.wantError)
			}
			if tt.wantUp && res.Error != "" {
				t.Errorf("Error = %q, want empty", res.Error)
			}
			if res.TargetID != 7 || res.URL != srv.URL || res.CheckedAt.IsZero() {
				t.Errorf("не заполнены поля результата: %+v", res)
			}
		})
	}
}

func TestCheck_ConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	res := NewWorker(time.Second).Check(context.Background(), models.Target{URL: url})
	if res.IsUp || res.StatusCode != 0 || res.Error == "" {
		t.Errorf("ожидалась сетевая ошибка, получено %+v", res)
	}
}

func TestCheck_DefaultTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	res := NewWorker(50*time.Millisecond).Check(context.Background(), models.Target{URL: srv.URL})
	if res.IsUp || !strings.Contains(res.Error, "таймаут: нет ответа за 50ms") {
		t.Errorf("ожидался таймаут по умолчанию, получено %+v", res)
	}
}

func TestCheck_TargetOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/created" {
			w.WriteHeader(http.StatusCreated)
		}
		w.Write([]byte("<h1>Привет, работодатель!</h1>"))
	}))
	defer srv.Close()

	headers := map[string]string{"apikey": "secret"}
	tests := []struct {
		name      string
		target    models.Target
		wantUp    bool
		wantError string
	}{
		{
			name:   "заголовки передаются",
			target: models.Target{URL: srv.URL, Headers: headers},
			wantUp: true,
		},
		{
			name:      "без заголовка — 401",
			target:    models.Target{URL: srv.URL},
			wantError: "неожиданный HTTP-код 401",
		},
		{
			name:   "ключевое слово найдено",
			target: models.Target{URL: srv.URL, Headers: headers, Keyword: "работодатель"},
			wantUp: true,
		},
		{
			name:      "ключевого слова нет",
			target:    models.Target{URL: srv.URL, Headers: headers, Keyword: "Internal Server Error"},
			wantError: "в ответе нет ключевого слова",
		},
		{
			name:   "ожидаемый код совпал",
			target: models.Target{URL: srv.URL + "/created", Headers: headers, ExpectedStatus: http.StatusCreated},
			wantUp: true,
		},
		{
			name:      "ожидался другой код",
			target:    models.Target{URL: srv.URL, Headers: headers, ExpectedStatus: http.StatusCreated},
			wantError: "неожиданный HTTP-код 200 (ожидался 201)",
		},
	}

	w := NewWorker(time.Second)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := w.Check(context.Background(), tt.target)
			if res.IsUp != tt.wantUp {
				t.Errorf("IsUp = %v, want %v (err: %q)", res.IsUp, tt.wantUp, res.Error)
			}
			if !strings.Contains(res.Error, tt.wantError) {
				t.Errorf("Error = %q, want contains %q", res.Error, tt.wantError)
			}
		})
	}
}
