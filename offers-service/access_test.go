package main

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateCodeShape(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code, err := generateCode()
		if err != nil {
			t.Fatalf("генерация кода: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("код должен быть из 6 цифр, получили %q", code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("в коде нецифровой символ: %q", code)
			}
		}
		seen[code] = true
	}
	if len(seen) < 90 {
		t.Fatalf("коды почти не повторяются: из 100 получили %d различных", len(seen))
	}
}

func TestVerifyCodeLogic(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name  string
		code  botCode
		input string
		want  string
	}{
		{
			name:  "истёк",
			code:  botCode{Code: "123456", ExpiresAt: now.Add(-time.Minute)},
			input: "123456",
			want:  "код истёк",
		},
		{
			name:  "использован",
			code:  botCode{Code: "123456", ExpiresAt: now.Add(time.Minute), UsedAt: &now},
			input: "123456",
			want:  "код уже использован",
		},
		{
			name:  "неверный",
			code:  botCode{Code: "123456", ExpiresAt: now.Add(time.Minute), Attempts: 2},
			input: "654321",
			want:  "неверный код",
		},
		{
			name:  "много попыток",
			code:  botCode{Code: "123456", ExpiresAt: now.Add(time.Minute), Attempts: 5},
			input: "123456",
			want:  "слишком много попыток",
		},
		{
			name:  "правильный",
			code:  botCode{Code: "123456", ExpiresAt: now.Add(time.Minute), Attempts: 1},
			input: "123456",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := verifyCodeLogic(tc.code, tc.input, now)
			if tc.want == "" && got != "" {
				t.Fatalf("ожидали успешную проверку, получили %q", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("ожидали ошибку %q, получили %q", tc.want, got)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := normalizeEmail("  Ivanov@TN.RU "); got != "ivanov@tn.ru" {
		t.Fatalf("ожидали нормализованный email, получили %q", got)
	}
	if got := normalizeEmail("не-почта"); got != "" {
		t.Fatalf("строка без @ должна отвергаться, получили %q", got)
	}
}

func TestDefaultRoleForNewUser(t *testing.T) {
	if got := defaultRoleForNewUser(); got != "viewer" {
		t.Fatalf("новый пользователь должен входить как viewer, получили %q", got)
	}
}
