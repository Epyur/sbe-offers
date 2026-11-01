package main

import (
	"context"
	"io"
	"net/http"
	"strings"
)

// kbImageSource — картинки карточек Базы знаний для документа (2026-11-01).
//
// Раньше документ собирался без фотографий продуктов: ImageSource был nil, и
// renderCardSections просто не вставлял картинки. Теперь ключ вложения
// превращается в ссылку (kb-service отдаёт presigned URL), а байты читаются по
// ней — второй способ доступа к тому же хранилищу не нужен.
//
// Ошибка чтения — не ошибка документа: картинка необязательна.

// KBImageSource — источник картинок поверх KBClient.
type KBImageSource struct {
	kb     *KBClient
	client *http.Client
}

// NewKBImageSource — адаптер для renderOfferDocxFull. kb == nil или kb не
// настроен — nil: документ собирается без картинок, как раньше.
func NewKBImageSource(kb *KBClient) ImageSource {
	if kb == nil || !kb.configured() {
		return nil
	}
	return &KBImageSource{kb: kb, client: &http.Client{Timeout: kbRequestTimeout}}
}

// Get читает картинку по ключу вложения.
func (s *KBImageSource) Get(ctx context.Context, key string) ([]byte, error) {
	if s == nil || s.kb == nil || strings.TrimSpace(key) == "" {
		return nil, errKBNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.client == nil {
		s.client = &http.Client{Timeout: kbRequestTimeout}
	}
	link, err := s.kb.FileLink(ctx, key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errKBNotConfigured
	}
	// Ограничение спасает от гигантского файла: в КП идут картинки продукта,
	// а не RAW со стенда.
	return io.ReadAll(io.LimitReader(resp.Body, 12<<20))
}
