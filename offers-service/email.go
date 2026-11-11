package main

import (
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// getEnv — короткая обёртка, чтобы не тянуть os в каждый файл.
func getEnv(key string) string {
	return os.Getenv(key)
}

// smtpSettings — разобранная конфигурация исходящей почты.
type smtpSettings struct {
	Host       string
	Port       string
	From       string
	User       string
	Pass       string
	SkipVerify bool
}

// smtpConfig читает окружение один раз, с дефолтами на внешний
// аутентифицированный провайдер. Причина перевода (2026-11-09, повтор
// 2026-11-12 для этого сервиса): письма через локальный exim с доменом
// epyur.fvds.ru (без SPF/DKIM/DMARC) корпоративный фильтр tn.ru кладёт в
// спам/карантин — коды входа в бота перестали доходить. Отправка через Яндекс
// (тот же ящик, что и у auth-service/documents-service): валидный сертификат,
// SPF/DKIM/DMARC, письмо шлётся «от себя».
//
// Явные SMTP_* по-прежнему главнее — откат на exim остаётся одной строкой в
// compose, без правки кода.
func smtpConfig() smtpSettings {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if host == "" {
		host = "smtp.yandex.ru"
	}
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if port == "" {
		port = "587"
	}
	user := strings.TrimSpace(os.Getenv("SMTP_USER"))
	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if from == "" {
		if user != "" {
			from = user
		} else {
			from = "noreply@epyur.fvds.ru"
		}
	}
	return smtpSettings{
		Host:       host,
		Port:       port,
		From:       from,
		User:       user,
		Pass:       os.Getenv("SMTP_PASS"),
		SkipVerify: os.Getenv("SMTP_SKIP_VERIFY") == "1",
	}
}

// buildMessage собирает MIME-сообщение: заголовки через CRLF, тема — по
// RFC 2047 (Q-кодирование), иначе кириллица в Subject выглядит для фильтров
// как мусор и повышает спам-оценку.
func buildMessage(from, to, subject, body string) string {
	return fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, to, mime.QEncoding.Encode("UTF-8", subject), body)
}

// sendEmail — отправка текстового письма (UTF-8) через SMTP. Транспорт
// зеркалит auth-service (email.go): явный DialTimeout + дедлайн на всю сессию
// (иначе зависшее соединение останавливает обработчик), STARTTLS если
// провайдер его анонсирует, AUTH PLAIN при заданных кредах.
func sendEmail(to, subject, body string) error {
	cfg := smtpConfig()
	msg := buildMessage(cfg.From, to, subject, body)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Now().Add(2 * time.Minute)); err != nil {
		_ = conn.Close()
		return err
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer c.Close()

	if err := c.Hello("offers-service"); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: cfg.Host, InsecureSkipVerify: cfg.SkipVerify}); err != nil {
			return err
		}
	}
	if cfg.User != "" && cfg.Pass != "" {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)); err != nil {
				return err
			}
		}
	}
	if err := c.Mail(cfg.From); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
