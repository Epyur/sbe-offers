package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"
)

// fakeImagesFor — источник картинок, который отдаёт PNG-заголовок для
// перечисленных ключей и «нет файла» для остальных.
//
// Отдельный от fakeImages (docx_card_test.go): тот годится для проверки
// описаний, но не различает ключи, а здесь важно, что разные узлы берут разные
// картинки.
type fakeImagesFor []string

func (f fakeImagesFor) Get(_ context.Context, key string) ([]byte, error) {
	for _, known := range f {
		if known == key {
			return []byte("\x89PNG\r\n\x1a\n fake png bytes for test " + key), nil
		}
	}
	return nil, errNoImage
}

var errNoImage = &noImageError{}

type noImageError struct{}

func (*noImageError) Error() string { return "нет картинки" }

// imagePartOf — первый файл word/media/* в собранном документе.
func imagePartOf(t *testing.T, data []byte) (string, bool) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("не zip: %v", err)
	}
	for _, f := range zr.File {
		if len(f.Name) > len("word/media/") && f.Name[:len("word/media/")] == "word/media/" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("открыть %s: %v", f.Name, err)
			}
			body, _ := io.ReadAll(rc)
			rc.Close()
			if len(body) == 0 {
				t.Fatalf("файл %s пуст", f.Name)
			}
			return f.Name, true
		}
	}
	return "", false
}
