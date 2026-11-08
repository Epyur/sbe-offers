package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Служебная метка системы внутри БЛОКА ФОРМЫ (2026-11-07).
//
// Живая жалоба на temple.docx: `{{система}}`, поставленная в блок формы КП и
// вставленная в макет меткой `{{block:…}}`, не печатала ничего — строки пустые.
// Причина не в данных, а в печати: узел абзаца не умеет отдать полотно Word
// (таблицу, картинку), поэтому `expandBlockServiceMarkers` оставлял от метки
// пустую строку, а в макете оставался пустой абзац.

// Блок формы с `{{система}}` печатает карточку: состав, таблицу без границ и
// картинку, а не пустой абзац.
func TestLayoutBlockWithSystemMarkerPrintsCard(t *testing.T) {
	var prompts []string
	lc := serviceLayoutContext(&prompts)
	lc.form = func() OfferForm {
		return OfferForm{Blocks: []Block{{
			ID:      "blk_1",
			Content: []byte(`[{"type":"paragraph","text":"{{система}}"}]`),
		}}}
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:blk_1}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "Мембрана Logicbase V-SL") {
		t.Fatalf("состав системы не напечатан:\n%s", body)
	}
	if n := strings.Count(body, "<w:tbl>"); n != 1 {
		t.Fatalf("таблиц %d, ожидалась одна:\n%s", n, body)
	}
	if !strings.Contains(body, "<w:drawing>") {
		t.Fatalf("картинка системы не напечатана:\n%s", body)
	}
	if strings.Contains(body, "</w:p><w:r><w:drawing>") {
		t.Fatalf("прогон картинки — сосед абзаца, Word её не покажет:\n%s", body)
	}
	if hasEmptyParagraph(body) {
		t.Fatalf("вместо блока остался пустой абзац:\n%s", body)
	}
	if !hasMediaPart(t, out) {
		t.Fatal("картинка не попала в пакет документа")
	}
	if len(prompts) != 0 {
		t.Fatalf("служебная метка позвала модель: %v", prompts)
	}
	if strings.Contains(body, "{{") {
		t.Fatalf("в документе осталась метка:\n%s", body)
	}
}

// Метка в тексте абзаца блока: текст сохраняется, карточка печатается следом.
func TestLayoutBlockSystemMarkerKeepsSurroundingText(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm {
		return OfferForm{Blocks: []Block{{
			ID:      "blk_1",
			Content: []byte(`[{"type":"paragraph","text":"Решение: {{система}}"}]`),
		}}}
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:blk_1}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "Решение:") {
		t.Fatalf("текст абзаца потерян:\n%s", body)
	}
	if !strings.Contains(body, "Мембрана Logicbase V-SL") {
		t.Fatalf("карточка не напечатана рядом с текстом:\n%s", body)
	}
}

// Метка `{{картинка}}` в абзаце блока печатает картинку системы.
//
// Проверка стережёт ту же ошибку, что была у `{{система}}` (2026-11-07): узел
// абзаца не умел отдать полотно, `resolvePlaceholders` вычищал скобки — и от
// метки оставался пустой абзац без картинки и без файла в пакете.
func TestLayoutBlockWithImageMarkerPrintsPicture(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm {
		return OfferForm{Blocks: []Block{{
			ID:      "blk_1",
			Content: []byte(`[{"type":"paragraph","text":"{{картинка}}"}]`),
		}}}
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:blk_1}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "<w:drawing>") {
		t.Fatalf("картинка не напечатана:\n%s", body)
	}
	if !hasMediaPart(t, out) {
		t.Fatal("картинка не попала в пакет документа")
	}
	if hasEmptyParagraph(body) {
		t.Fatalf("вместо картинки остался пустой абзац:\n%s", body)
	}
	if strings.Contains(body, "{{") {
		t.Fatalf("метка ушла клиенту литералом:\n%s", body)
	}
}

// Картинка карточки — прогон ВНУТРИ абзаца. Голый `<w:r>` рядом с `</w:p>`
// Word не показывает вовсе: это невалидный OOXML.
func TestLayoutCardImageMarkupWrapsRunInParagraph(t *testing.T) {
	lc := layoutContext{
		images: &fakeImages{data: pngBytes()},
		layout: &layoutRuntime{media: newDocxMedia(nil)},
	}
	markup := layoutCardImageMarkup(SystemCard{ImageKey: "k/1", ImageName: "schema.png"}, lc)
	if markup == "" {
		t.Fatal("картинка карточки не собрана")
	}
	drawing := strings.Index(markup, "<w:drawing>")
	open := strings.Index(markup, "<w:p")
	closeP := strings.Index(markup, "</w:p>")
	if open != 0 || !strings.HasSuffix(markup, "</w:p>") {
		t.Fatalf("картинка не обёрнута абзацем: %q", markup)
	}
	if drawing < open || drawing > closeP || closeP < 0 {
		t.Fatalf("прогон картинки оказался вне абзаца: %q", markup)
	}
	if strings.Contains(markup, "</w:p><w:r") {
		t.Fatalf("прогон картинки — сосед абзаца, а не его содержимое: %q", markup)
	}
}

// Кеш систем, прочитанный ДО появления ImageKey, обязан получить ключ картинки
// при чтении: иначе метка системы молчит про картинку навсегда.
func TestSystemsFromKBAttachesImageKeys(t *testing.T) {
	kb := &fakeKB{
		tagID:       7,
		notes:       []KBNoteSummary{{ID: 5, Title: "Карточка", Path: "База/ТН.md"}},
		note:        sampleCardNote(),
		attachments: []KBAttachment{{Filename: "pic.png", Mime: "image/png", Key: "att/pic.png"}},
	}
	cards, err := SystemsFromKB(context.Background(), kb, nil)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("ожидали одну карточку, получили %d", len(cards))
	}
	if cards[0].ImageKey != "att/pic.png" {
		t.Fatalf("ключ картинки не привязан: %q", cards[0].ImageKey)
	}
}

// Выборка карточек предложения тоже обязана привязать картинку: кеш на живой
// базе хранит `imageName` и пустой `imageKey`, и без этого шага документ
// печатает систему без картинки.
func TestOfferCardsWithImagesFillsSelectedCard(t *testing.T) {
	kb := &fakeKB{
		attachments: []KBAttachment{{Filename: "pic.png", Mime: "image/png", Key: "att/pic.png"}},
	}
	cards := []SystemCard{
		{NoteID: 5, ImageName: "pic.png"},
		{NoteID: 6, ImageName: "other.png"},
	}
	offer := Offer{Systems: []OfferSystem{{NoteID: 5}}}
	got := offerCardsWithImages(context.Background(), kb, cards, offer)
	if len(got) != 1 {
		t.Fatalf("выбрана не одна карточка: %+v", got)
	}
	if got[0].ImageKey != "att/pic.png" {
		t.Fatalf("картинка выбранной системы не привязана: %q", got[0].ImageKey)
	}
}

// hasEmptyParagraph — в документе есть абзац без единого прогона.
func hasEmptyParagraph(body string) bool {
	return strings.Contains(body, "</w:pPr></w:p>") || strings.Contains(body, "<w:p/>")
}

// Живой макет пользователя (temple.docx): метка системы и в БЛОКЕ формы, и
// напрямую в макете (2026-11-07).
//
// Проверка на настоящем файле потому, что в нём метка `{{block:…}}` разрезана
// Word на несколько прогонов — ровно так, как её увидит служба на бою. Обе
// дороги обязаны дать одно и то же: таблицу состава и картинку.
//
//	OFFERS_LIVE_LAYOUT — путь к temple.docx (в репозитории его нет).
func TestLiveTempleLayoutPrintsBlockAndDirectSystem(t *testing.T) {
	master := os.Getenv("OFFERS_LIVE_LAYOUT")
	if master == "" {
		t.Skip("OFFERS_LIVE_LAYOUT не задан")
	}
	layout, err := os.ReadFile(master)
	if err != nil {
		t.Fatalf("чтение макета: %v", err)
	}
	// Id блока — живой, из temple.docx: проверка обязана попасть в ту метку,
	// которую поставил администратор, а не в придуманную.
	const blockID = "blk_mw6oonck8g8z"
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm {
		return OfferForm{Blocks: []Block{{
			ID:      blockID,
			Content: []byte(`[{"type":"paragraph","text":"{{система}}"}]`),
		}}}
	}
	out, err := renderLayoutDocx(layout, lc)
	if err != nil {
		t.Fatalf("печать по макету: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))

	// Строка состава — из карточки стенда (serviceLayoutContext). В живой
	// карточке она набрана с кавычками вокруг марок, но здесь проверяется
	// МАКЕТ, и зависимость от формата карточки сделала бы тест хрупким.
	const compositionLine = "Мембрана Logicbase V-SL"
	if n := strings.Count(body, compositionLine); n != 1 {
		t.Fatalf("состав напечатан %d раз, ожидалась одна таблица системы:\n%s", n, body)
	}
	if n := strings.Count(body, "<w:drawing>"); n != 1 {
		t.Fatalf("картинок %d, ожидалась одна:\n%s", n, body)
	}
	if strings.Contains(body, "</w:p><w:r>") {
		t.Fatalf("прогон оказался соседом абзаца, Word его не покажет:\n%s", body)
	}
	if !hasZipPart(t, out, "word/media/offerimg1.png") {
		t.Fatal("в пакете нет word/media/offerimg1.png")
	}
	for _, bad := range []string{"{{", "}}"} {
		if strings.Contains(body, bad) {
			t.Fatalf("в документе остались скобки метки %q:\n%s", bad, body)
		}
	}
}

// hasZipPart — есть ли часть с таким именем в собранном пакете.
func hasZipPart(t *testing.T, docx []byte, name string) bool {
	t.Helper()
	for _, part := range zipReaderOf(t, docx) {
		if part.name == name {
			return true
		}
	}
	return false
}
