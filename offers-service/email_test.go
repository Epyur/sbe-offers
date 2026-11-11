package main

import (
	"strings"
	"testing"
)

// clearSMTPEnv — окружение без SMTP_*, чтобы дефолты проверялись честно.
func clearSMTPEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_FROM", "SMTP_USER", "SMTP_PASS", "SMTP_SKIP_VERIFY"} {
		t.Setenv(k, "")
	}
}

// TestSmtpConfigDefaultsToYandex — без SMTP_* транспорт обязан идти через
// внешний аутентифицированный провайдер, а не через локальный exim
// (2026-11-12: коды входа в бота перестали доходить через exim).
func TestSmtpConfigDefaultsToYandex(t *testing.T) {
	clearSMTPEnv(t)
	cfg := smtpConfig()
	if cfg.Host != "smtp.yandex.ru" || cfg.Port != "587" {
		t.Fatalf("default transport = %s:%s, want smtp.yandex.ru:587", cfg.Host, cfg.Port)
	}
	if cfg.From != "noreply@epyur.fvds.ru" {
		t.Fatalf("default sender = %q", cfg.From)
	}
}

// TestSmtpConfigFromFallsBackToUser — адрес отправителя без SMTP_FROM берётся
// из логина провайдера: письмо шлётся «от себя», иначе SPF/DKIM не сходятся.
func TestSmtpConfigFromFallsBackToUser(t *testing.T) {
	clearSMTPEnv(t)
	t.Setenv("SMTP_USER", "lpitn@yandex.ru")
	t.Setenv("SMTP_PASS", "secret")
	cfg := smtpConfig()
	if cfg.From != "lpitn@yandex.ru" {
		t.Fatalf("sender = %q, want SMTP_USER", cfg.From)
	}
	if cfg.User != "lpitn@yandex.ru" || cfg.Pass != "secret" {
		t.Fatal("credentials must be read from SMTP_USER/SMTP_PASS")
	}
}

// TestSmtpConfigHonoursExplicitOverride — явные SMTP_* управляют транспортом:
// откат на exim остаётся одной строкой в compose.
func TestSmtpConfigHonoursExplicitOverride(t *testing.T) {
	clearSMTPEnv(t)
	t.Setenv("SMTP_HOST", "host.docker.internal")
	t.Setenv("SMTP_PORT", "25")
	t.Setenv("SMTP_FROM", "noreply@epyur.fvds.ru")
	t.Setenv("SMTP_SKIP_VERIFY", "1")
	cfg := smtpConfig()
	if cfg.Host != "host.docker.internal" || cfg.Port != "25" {
		t.Fatalf("transport = %s:%s, want explicit override", cfg.Host, cfg.Port)
	}
	if cfg.From != "noreply@epyur.fvds.ru" {
		t.Fatalf("sender = %q", cfg.From)
	}
	if !cfg.SkipVerify {
		t.Fatal("SkipVerify must be true when SMTP_SKIP_VERIFY=1")
	}
}

// TestBuildMessageEncodesSubject — кириллица в теме кодируется по RFC 2047,
// сырая кириллица в Subject повышает спам-оценку.
func TestBuildMessageEncodesSubject(t *testing.T) {
	msg := buildMessage("a@tn.ru", "b@tn.ru", "Код входа", "тело")
	if strings.Contains(msg, "Subject: Код") {
		t.Fatal("subject must be RFC 2047 encoded")
	}
	if !strings.Contains(msg, "Subject: =?UTF-8?q?") {
		t.Fatalf("subject encoding missing: %q", msg)
	}
	if !strings.Contains(msg, "\r\n\r\nтело") {
		t.Fatal("body must follow the header block")
	}
}

// TestBuildMessageCarriesRecipientAndBody — письмо с кодом доходит до
// получателя с телом в UTF-8.
func TestBuildMessageCarriesRecipientAndBody(t *testing.T) {
	msg := buildMessage("a@tn.ru", "b@tn.ru", "Код входа в бота Коммерческие предложения", "Код: 123456")
	if !strings.Contains(msg, "To: b@tn.ru") {
		t.Fatalf("recipient missing: %q", msg)
	}
	if !strings.Contains(msg, "Код: 123456") {
		t.Fatalf("body missing: %q", msg)
	}
}
