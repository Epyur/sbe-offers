package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// liveTempleLayout читает живой макет по переменной окружения.
//
// Сам файл в репозиторий не попадает: в нём данные заказчика. Без переменной
// тест пропускается — правило «живой документ не коммитим» (2026-11-02).
func liveTempleLayout(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("OFFERS_LIVE_LAYOUT")
	if path == "" {
		t.Skip("OFFERS_LIVE_LAYOUT не задана")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("макет не прочитан: %v", err)
	}
	return data
}

// Сквозная проверка по ЖИВОМУ макету `temple.docx` (2026-11-07).
//
// Пользователь сообщил: `{{текст: система: description}}` не работает «ни через
// прямую вставку в макет, ни через вставку в блоки форм». Причина найдена и
// замерена — сохранённых текстов у системы нет (`offers_system_text` пуста), а
// метка печатает ТОЛЬКО сохранённое. Отсюда второе требование: сборка документа
// должна дописывать недостающие тексты сама, иначе метка молчит всегда, сколько
// бы её ни ставили.
//
// Этот тест держит обе границы на настоящем файле: составные метки из живого
// макета не должны оставаться литералом ни в одном из путей.
func TestLiveTempleLayoutHasNoLiteralMarkers(t *testing.T) {
	layout := liveTempleLayout(t)
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm {
		return OfferForm{Blocks: []Block{
			{ID: "blk_mw6oonck8g8z", Title: "Описание",
				Content: []byte(`[{"type":"paragraph","text":"{{текст: система: description}}"}]`)},
		}}
	}
	lc.systemTexts = func(SystemCard) map[string]string {
		return map[string]string{
			systemTextDescription: "Обоснование применения по карточке.",
			systemTextAdvantages:  "Преимущества по карточке.",
		}
	}
	out, err := renderLayoutDocx(layout, lc)
	if err != nil {
		t.Fatalf("печать по живому макету: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	for _, bad := range []string{"{{текст", "{{block", "{{система", "{{", "}}"} {
		if bytes.Contains([]byte(body), []byte(bad)) {
			t.Errorf("метка %q дошла литералом до документа", bad)
		}
	}
	// Текст системы напечатан дважды: метка в макете и блок формы.
	n := bytes.Count([]byte(body), []byte("Обоснование применения по карточке."))
	if n != 2 {
		t.Errorf("текст системы напечатан %d раз, ожидалось два (макет + блок)", n)
	}
}

// Живой случай: сохранённых текстов нет, и метка печатала ПУСТО (2026-11-07).
//
// Метка обязана дополнительно позвать модель и записать ответ — иначе она не
// работает никогда, пока человек не нажмёт «Пересчитать». Здесь это одно
// требование: подписи из макета печатаются без литерала, а при пустом ответе
// модели место остаётся пустым и об этом пишется в журнал.
func TestSystemTextMarkerAsksModelWhenNothingStored(t *testing.T) {
	var prompts []string
	lc := serviceLayoutContext(&prompts)
	lc.systemTexts = func(SystemCard) map[string]string { return nil }
	// Дописывает текст системы тот же вызывающий, что и в бою: путь печати сам
	// это не решает — у него нет ни модели, ни БД.
	lc.systemText = func(card SystemCard, block string) string {
		return lc.prompt("Обоснование применения системы: " + block)
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{текст: система: description}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if len(prompts) != 1 {
		t.Fatalf("модель вызвана %d раз, ожидался один: %v", len(prompts), prompts)
	}
	if !strings.Contains(prompts[0], "Обоснование применения") {
		t.Errorf("модели передана не та просьба: %q", prompts[0])
	}
	if !strings.Contains(body, "ТЕКСТ ОТ МОДЕЛИ") {
		t.Fatalf("ответ модели не напечатан:\n%s", body)
	}
}
