package main

import (
	"archive/zip"
	"bytes"
	"context"
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
)

// layoutModeFrom — режим формы, к которому относится запрос макета.
//
// `mode` в запросе: `client` (обычное КП) или `combat` (боевой листок). Всё
// остальное — клиентский документ: режим — короткая строка, и опечатка в ней не
// должна уносить макет в неизвестную форму.
func layoutModeFrom(r *http.Request) string {
	if strings.TrimSpace(r.URL.Query().Get("mode")) == combatFormMode {
		return combatFormMode
	}
	return clientFormMode
}

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
	// Режим берётся из запроса (2026-11-11): у обычного КП макет свой, у боевого
	// листка — свой. До этого режим был зашит в «client», и боевой листок
	// загрузить макет не мог вовсе.
	mode := layoutModeFrom(r)
	if _, err := s.pool.Exec(r.Context(), `
UPDATE offers_form SET layout_docx = $2, layout_name = $3, updated_at = now() WHERE mode = $1`,
		mode, body, name); err != nil {
		dbError(w, "upload layout", err)
		return
	}
	fields, prompts := describeLayout(body)
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":          mode,
		"hasLayout":     true,
		"layoutName":    name,
		"layoutFields":  fields,
		"layoutPrompts": prompts,
		"bytes":         len(body),
	})
}

// uploadLayoutResponse — ответ на загрузку макета.
//
// Ответ называет РЕЖИМ, в который попал файл (2026-11-11): иначе загрузка
// макета в чужой документ выглядит успехом, и об этом узнают только при
// печати — когда вместо КП выходит пустой макет другого документа. Тем же
// ответом пользуются тесты: судить надо то, что видит клиент.
func uploadLayoutResponse(mode string, body []byte, name string) map[string]any {
	fields, prompts := describeLayout(body)
	return map[string]any{
		"mode":          mode,
		"hasLayout":     true,
		"layoutName":    name,
		"layoutFields":  fields,
		"layoutPrompts": prompts,
		"bytes":         len(body),
	}
}

// handleDeleteLayout убирает макет: печать возвращается к блокам формы.
func (s *Server) handleDeleteLayout(w http.ResponseWriter, r *http.Request) {
	if _, err := s.pool.Exec(r.Context(), `
UPDATE offers_form SET layout_docx = NULL, layout_name = '', updated_at = now() WHERE mode = $1`,
		layoutModeFrom(r)); err != nil {
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
		layoutModeFrom(r)).Scan(&data, &name); err != nil || len(data) == 0 {
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

// handleExampleLayout отдаёт рабочий образец макета со всеми метками.
//
// Требование пользователя 2026-11-06 (шаг E): инструкция по разметке должна
// начинаться с рабочего образца, а не со слов. Образец собирается ЗДЕСЬ, из
// того же кода, что и печать, — поэтому он не разойдётся с системой: новая
// метка добавляется в один список `layoutExampleMarkers`.
func (s *Server) handleExampleLayout(w http.ResponseWriter, r *http.Request) {
	data, err := exampleLayoutDocx()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	name := "образец-макета-КП.docx"
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.QueryEscape(name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// layoutExampleMarker — строка образца: подпись и сама метка.
type layoutExampleMarker struct {
	label  string
	marker string
}

// layoutExampleMarkers — все метки, которые понимает печать.
//
// Один список на справку и на образец: метка, которой здесь нет, в образец не
// попадёт, а новая метка попадает сюда в том же изменении, что и разбор.
var layoutExampleMarkers = []layoutExampleMarker{
	{"Номер предложения", "[Номер предложения]"},
	{"Дата предложения", "[Дата предложения]"},
	{"Имя менеджера", "[Имя менеджера]"},
	{"Получатель", "[Получатель из КП]"},
	{"Объект", "[Наименование объекта]"},
	{"Картинка и состав системы одной таблицей", "{{система}}"},
	{"Обоснование применения (текст сохранён при выборе системы)", "{{текст: система: description}}"},
	{"Преимущества (текст сохранён при выборе системы)", "{{текст: система: advantages}}"},
	{"Ссылки карточки", "{{ссылки: система}}"},
	{"Характеристики карточки", "{{характеристики: система}}"},
	{"Блок формы по его id", "{{block:1}}"},
	{"Текст модели по подсказке", "{Опиши систему в двух абзацах}"},
}

// exampleLayoutDocx собирает минимальный .docx со всеми метками.
//
// Стили бланка не нужны: образец — про разметку, а не про оформление. Каждая
// метка стоит отдельным абзацем, подпись жирным — человек видит, что к чему.
func exampleLayoutDocx() ([]byte, error) {
	var body strings.Builder
	body.WriteString(paragraphXML("Образец макета КП — LogicTEAM.КП", paraOpts{}))
	for _, item := range layoutExampleMarkers {
		body.WriteString(paragraphXML(item.label+":", paraOpts{}))
		body.WriteString(paragraphXML(item.marker, paraOpts{}))
	}
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` + body.String() + `</w:body></w:document>`
	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`</Types>`),
		"_rels/.rels": []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`),
		"word/document.xml": []byte(doc),
	}
	names := []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"}
	return zipFiles(files, names)
}

// loadLayoutForMode — байты макета формы по режиму.
//
// Отдельно от `loadLayout`: тот берёт режим из запроса (админ грузит и
// скачивает макет), а печать работает сама, без запроса, и ей нужен макет
// своего режима. nil, nil — макета нет, это не ошибка.
func (s *Server) loadLayoutForMode(ctx context.Context, mode string) ([]byte, error) {
	var data []byte
	err := s.pool.QueryRow(ctx,
		`SELECT layout_docx FROM offers_form WHERE mode = $1`, mode).Scan(&data)
	if err != nil || len(data) == 0 {
		return nil, nil
	}
	return data, nil
}

// loadLayout читает макет формы. nil, nil — макета нет, печать по блокам.
func (s *Server) loadLayout(r *http.Request) ([]byte, error) {
	var data []byte
	err := s.pool.QueryRow(r.Context(),
		`SELECT layout_docx FROM offers_form WHERE mode = $1`, layoutModeFrom(r)).Scan(&data)
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
