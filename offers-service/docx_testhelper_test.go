package main

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

// docXMLOf — содержимое word/document.xml из собранного DOCX (для тестов формы).
func docXMLOf(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("не zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("открыть document.xml: %v", err)
			}
			raw, _ := io.ReadAll(rc)
			rc.Close()
			return string(raw)
		}
	}
	t.Fatal("в документе нет word/document.xml")
	return ""
}
