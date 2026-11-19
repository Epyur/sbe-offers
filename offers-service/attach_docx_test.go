package main

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

// Word как источник контекста (2026-11-18, пункт backlog «.docx как источник
// контекста»).
//
// Старые КП приходят в .docx чаще, чем в Excel, а до этой правки файл
// отклонялся словами «файлы Word пока не принимаю». Сторожа держат три вещи:
// текст Word доходит до брифа, пустой файл (скан без текстового слоя) получает
// честный отказ, а у разбора есть названная граница — состав позиций из Word
// не собирается, и обещать его нельзя.

// makeDocx собирает минимальный .docx: один word/document.xml, абзацы w:p.
//
// Настоящий Word пишет ещё [Content_Types].xml и _rels, но типу файла хватает
// одного word/document.xml — ровно по нему DetectAttachmentType и различает
// ooxml (zipKind).
func makeDocx(t *testing.T, paragraphs []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("создать запись архива: %v", err)
	}
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	body.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, p := range paragraphs {
		body.WriteString(`<w:p><w:r><w:t>`)
		body.WriteString(xmlEscape(p))
		body.WriteString(`</w:t></w:r></w:p>`)
	}
	body.WriteString(`</w:body></w:document>`)
	if _, err := w.Write([]byte(body.String())); err != nil {
		t.Fatalf("записать document.xml: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("закрыть архив: %v", err)
	}
	return buf.Bytes()
}

// xmlEscape — минимум, без которого XML не читается: & < >.
func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func TestReadAttachedFileReadsWordAsContext(t *testing.T) {
	// Модель в тесте выключена: проверяется дословный разбор, без сети.
	t.Setenv("LLM_SERVICE_SECRET", "")
	srv := &Server{}
	data := makeDocx(t, []string{
		"Коммерческое предложение № 012/2025",
		"Объект: ЖК «Поэма», корпус 3",
		"Заказчик: ООО Строй-Инвест",
		"Сроки: до 15 сентября",
	})

	out, err := srv.readAttachedFile(context.Background(), Offer{Number: "001/2026"}, "старое КП.docx", data)
	if err != nil {
		t.Fatalf("Word отклонён: %v", err)
	}
	if out.Kind != "docx" {
		t.Fatalf("вид файла = %q, ждали docx", out.Kind)
	}
	text := strings.Join(out.Context, "\n")
	if !strings.Contains(text, "Поэма") {
		t.Fatalf("текст Word не попал в контекст: %q", text)
	}
	if out.ContextFields == nil || !out.ContextFields.HasContent() {
		t.Fatalf("контекст объекта не разложен: %#v", out.ContextFields)
	}
	if got := out.ContextFields.Fields["object_name"]; len(got) == 0 || !strings.Contains(got[0], "Поэма") {
		t.Fatalf("объект в полях = %#v", out.ContextFields.Fields)
	}
	if !out.NeedsConfirm {
		t.Fatal("файл прочитан, а подтверждения не просят")
	}
	if !strings.Contains(out.Reply, "бриф") {
		t.Fatalf("не сказано, куда пойдёт разбор: %q", out.Reply)
	}
}

func TestReadAttachedFileWordWithoutTextSaysSo(t *testing.T) {
	t.Setenv("LLM_SERVICE_SECRET", "")
	srv := &Server{}
	data := makeDocx(t, nil)

	_, err := srv.readAttachedFile(context.Background(), Offer{Number: "001/2026"}, "скан.docx", data)
	if err == nil {
		t.Fatal("пустой Word принят молча")
	}
	if !strings.Contains(err.Error(), "нет текста") {
		t.Fatalf("отказ не объясняет причину: %v", err)
	}
	if strings.Contains(err.Error(), "не принимаю") {
		t.Fatalf("Word по-прежнему отклоняется целиком: %v", err)
	}
}

func TestAttachReplyWordWithoutFactsNamesTheFormat(t *testing.T) {
	// Word прочитан (текст есть), но описания объекта в нём не нашлось.
	// Ответ обязан сказать две вещи: файл прочитан и состав из Word не
	// разбирается — иначе менеджер ждёт позиции, которых не будет.
	reply := attachReply(&attachedFile{
		Kind:    "docx",
		Name:    "старое КП.docx",
		Context: []string{"Коммерческое предложение № 012/2025", "С уважением, отдел продаж"},
	}, "старое КП.docx")
	if !strings.Contains(reply, "Word") {
		t.Fatalf("не сказано, что прочитан Word: %q", reply)
	}
	// Регистр не важен: проверяется сам факт названной границы, а не заглавная
	// буква в начале фразы.
	if !strings.Contains(strings.ToLower(reply), "состав") {
		t.Fatalf("не названа граница «состав из Word не разбираю»: %q", reply)
	}
}
