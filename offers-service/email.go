package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
)

// getEnv — короткая обёртка, чтобы не тянуть os в каждый файл.
func getEnv(key string) string {
	return os.Getenv(key)
}

// sendEmail отправляет письмо через локальный exim (SMTP). Конфиг из env:
// SMTP_HOST/SMTP_PORT/SMTP_FROM/SMTP_USER/SMTP_PASS/SMTP_SKIP_VERIFY.
func sendEmail(to, subject, body string) error {
	host := getEnv("SMTP_HOST")
	if host == "" {
		host = "localhost"
	}
	port := getEnv("SMTP_PORT")
	if port == "" {
		port = "25"
	}
	from := getEnv("SMTP_FROM")
	if from == "" {
		from = "noreply@epyur.fvds.ru"
	}
	user := getEnv("SMTP_USER")
	pass := getEnv("SMTP_PASS")
	skipVerify := getEnv("SMTP_SKIP_VERIFY") == "1"

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, to, subject, body)

	addr := fmt.Sprintf("%s:%s", host, port)

	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Hello("offers-service"); err != nil {
		return err
	}

	if ok, _ := c.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{ServerName: host, InsecureSkipVerify: skipVerify}
		if err := c.StartTLS(tlsConfig); err != nil {
			return err
		}
	}

	if user != "" {
		auth := smtp.PlainAuth("", user, pass, host)
		if err := c.Auth(auth); err != nil {
			return err
		}
	}

	if err := c.Mail(from); err != nil {
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
	if err := c.Quit(); err != nil {
		log.Printf("sendEmail: quit: %v", err)
	}
	return nil
}

// ensure strings используется, даже если шаблон письма поменяется.
var _ = strings.TrimSpace
