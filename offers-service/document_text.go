package main

// Извлечение текста из файла документа для агента (2026-11-14, шаг P6).
//
// Честная граница v1: текст извлекается из DOCX (это zip с XML — разбор без
// зависимостей) и из простых текстовых файлов. PDF в v1 текстом НЕ читается:
// библиотек PDF в сервисе нет, а добавлять новую зависимость ради одного
// инструмента — отдельное решение с проверкой лицензии. Поэтому PDF-скан
// честно даёт пустой текст, а агент откроет документ ссылкой
// (`get_document_link`) — это ровно тот путь, ради которого он и есть.
//
// Что здесь НЕ делать: не возвращать base64 картинки, не OCR-ить. Модель не
// отличит base64 от мусора, а скан без слоя не превращается в текст сам.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"strings"
)

// documentTextOf — текст файла по его ключу (расширение) и байтам.
//
// Пустой текст — законный ответ: скан без слоя, картинка, бинарник. Ошибку
// разбора тоже отдаём пустым текстом: агенту важнее, что файл нашёлся, а
// значит его можно открыть ссылкой, чем точная причина, почему не разобрался.
func documentTextOf(key string, data []byte) string {
	switch strings.ToLower(path.Ext(strings.TrimSpace(key))) {
	case ".docx", ".dotx":
		return docxBodyText(data)
	case ".txt", ".md", ".csv", ".json", ".html", ".htm", ".xml":
		return strings.TrimSpace(string(data))
	}
	return ""
}

// docxBodyText — текст тела документа Word.
//
// Разбор `word/document.xml`: берём текстовые узлы `w:t`, абзацы (`w:p`)
// разделяем переводом строки, вкладку (`w:tab`) — пробелом. Это не полноценная
// типографика (сноски, таблицы отдельными блоками не выделяются), но для
// «прочитать протокол» достаточно, а лишние зависимости не нужны.
func docxBodyText(data []byte) string {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return ""
		}
		raw, err := io.ReadAll(io.LimitReader(rc, 8<<20))
		rc.Close()
		if err != nil {
			return ""
		}
		return wordXMLText(raw)
	}
	return ""
}

// wordXMLText — текстовые узлы `w:t` из XML документа Word.
func wordXMLText(raw []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var out strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch node := token.(type) {
		case xml.StartElement:
			switch node.Name.Local {
			case "t":
				text, err := xmlText(decoder)
				if err == nil {
					out.WriteString(text)
				}
			case "tab":
				out.WriteString(" ")
			case "br", "cr":
				out.WriteString("\n")
			}
		case xml.EndElement:
			if node.Name.Local == "p" {
				out.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(out.String())
}

// xmlText — текстовое содержимое текущего элемента до его закрытия.
func xmlText(decoder *xml.Decoder) (string, error) {
	var out strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return out.String(), err
		}
		switch node := token.(type) {
		case xml.CharData:
			out.Write(node)
		case xml.EndElement:
			return out.String(), nil
		}
	}
}
