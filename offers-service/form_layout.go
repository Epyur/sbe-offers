package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Макет КП (.docx) — загрузка, чтение, удаление (2026-11-05).
//
// Пользователь просил: «пользователь мог бы загрузить в систему макет ТКП,
// где он мог бы заполнить два типа плейсхолдеров». Макет — обычный .docx с
// метками `[…]` (жёсткая подстановка) и `{…}` (промт модели).
//
// Файл принимается телом запроса (не multipart): плагин и так передаёт байты
// через `requestUrl`, а разбор multipart в сервисе ради одного файла — лишний
// код и лишняя возможность ошибиться.

const (
	// maxLayoutBytes — предел размера макета. КП в Word — десятки килобайт;
	// 12 МБ хватает и на картинки внутри документа, а больше — это уже не
	// макет, а повод проверить, что загружают.
	maxLayoutBytes = 12 << 20
	// layoutMode — режим формы, к которому относится макет. Один на всё:
	// клиентский документ. Внутренний режим макетом не печатается.
	layoutMode = "client"
)

// handleUploadLayout принимает .docx и сохраняет его как макет формы.
func (s *Server) handleUploadLayout(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxLayoutBytes))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"error": "макет больше 12 МБ",
		})
		return
	}
	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "пустой файл"})
		return
	}
	if err := validateLayoutDocx(body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "макет.docx"
	}
	if _, err := s.pool.Exec(r.Context(), `
UPDATE offers_form SET layout_docx = $2, layout_name = $3, updated_at = now() WHERE mode = $1`,
		layoutMode, body, name); err != nil {
		dbError(w, "upload layout", err)
		return
	}
	fields, prompts := describeLayout(body)
	writeJSON(w, http.StatusOK, map[string]any{
		"hasLayout":     true,
		"layoutName":    name,
		"layoutFields":  fields,
		"layoutPrompts": prompts,
		"bytes":         len(body),
	})
}

// handleDeleteLayout убирает макет: печать возвращается к блокам формы.
func (s *Server) handleDeleteLayout(w http.ResponseWriter, r *http.Request) {
	if _, err := s.pool.Exec(r.Context(), `
UPDATE offers_form SET layout_docx = NULL, layout_name = '', updated_at = now() WHERE mode = $1`,
		layoutMode); err != nil {
		dbError(w, "delete layout", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hasLayout": false})
}

// handleDownloadLayout отдаёт загруженный макет — чтобы человек мог скачать
// свой файл и поправить метки, не начиная заново.
func (s *Server) handleDownloadLayout(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var name string
	if err := s.pool.QueryRow(r.Context(),
		`SELECT layout_docx, layout_name FROM offers_form WHERE mode = $1`,
		layoutMode).Scan(&data, &name); err != nil || len(data) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "макет не загружен"})
		return
	}
	if strings.TrimSpace(name) == "" {
		name = "макет.docx"
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.QueryEscape(name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// loadLayout читает макет формы. nil, nil — макета нет, печать по блокам.
func (s *Server) loadLayout(r *http.Request) ([]byte, error) {
	var data []byte
	err := s.pool.QueryRow(r.Context(),
		`SELECT layout_docx FROM offers_form WHERE mode = $1`, layoutMode).Scan(&data)
	if err != nil {
		// Строки нет или макет пуст — это не ошибка, а «макета нет».
		return nil, nil
	}
	if len(data) == 0 {
		return nil, nil
	}
	return data, nil
}

// validateLayoutDocx — файл действительно .docx: zip, в нём есть document.xml.
//
// Проверка до записи: иначе человек загрузит старый .doc (не zip) и узнает об
// этом при печати КП, когда менеджер уже ждёт файл.
func validateLayoutDocx(data []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return errors.New("файл не похож на документ Word (.docx): " +
			"старый .doc не подойдёт — сохраните как «Документ Word (*.docx)»")
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			return nil
		}
	}
	return errors.New("в файле нет word/document.xml — это не документ Word")
}
