package main

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

// Части собранного .docx для проверок.

// zipPart — одна часть пакета.
type zipPart struct {
	name string
	data []byte
}

// zipReaderOf — все части .docx по порядку.
func zipReaderOf(t *testing.T, docx []byte) []zipPart {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("не zip: %v", err)
	}
	out := make([]zipPart, 0, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("открыть %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("прочитать %s: %v", f.Name, err)
		}
		out = append(out, zipPart{name: f.Name, data: data})
	}
	return out
}

// zipOf — .docx из набора частей (порядок — как передали имена, чтобы
// [Content_Types].xml не оказался в середине пакета).
func zipOf(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	files := make(map[string][]byte, len(parts))
	for name, body := range parts {
		files[name] = []byte(body)
	}
	names := make([]string, 0, len(parts))
	if _, ok := parts["[Content_Types].xml"]; ok {
		names = append(names, "[Content_Types].xml")
	}
	for name := range parts {
		if name != "[Content_Types].xml" {
			names = append(names, name)
		}
	}
	data, err := zipFiles(files, names)
	if err != nil {
		t.Fatalf("собрать пакет: %v", err)
	}
	return data
}
