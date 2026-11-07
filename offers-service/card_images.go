package main

import (
	"context"
	"strings"
)

// Картинки систем и материалов для документа (2026-11-05).
//
// Требование пользователя: в описании конкретного блока должно быть видно, что
// сюда вставляется картинка системы и/или материала. До этого картинка была
// возможна только у системы, а её ключ (`ImageKey`) в карточке НИКОГДА не
// заполнялся: `ParseSystemCard` отдаёт имя файла из `![[…]]`, а ключ вложения
// в хранилище — другое дело, и связь между ними никто не устанавливал. У
// материалов картинки не было вовсе, хотя колонка `PhotoKey` в базе есть.
//
// Здесь одна точка привязки: имя файла из заметки → вложение карточки → ключ
// вложения. Ключ нужен, чтобы `KBImageSource` (уже существующий) нашёл байты.

// attachImageKeys проставляет карточкам ключ картинки по её вложению.
//
// Ошибка получения вложений не прерывает чтение карточек: картинка — не
// обязательная часть документа, и её отсутствие не должно прятать систему.
// Вложение выбирается по имени из `![[…]]`; если имени нет или точного
// совпадения нет — первая картинка из вложений: в карточке обычно ровно одна,
// и «нет картинки» там, где она есть, — хуже, чем «картинка не та».
func attachImageKeys(ctx context.Context, kb KBReader, cards []SystemCard) []SystemCard {
	if kb == nil || len(cards) == 0 {
		return cards
	}
	for i := range cards {
		attachImageKey(ctx, kb, &cards[i])
	}
	return cards
}

// attachImageKey — ключ картинки одной карточки.
//
// Пропускается всё, где привязывать нечего или не к чему: ключ уже есть, нет
// заметки, ошибка вложений (сеть), нет картинок среди вложений.
func attachImageKey(ctx context.Context, kb KBReader, card *SystemCard) {
	if card.ImageKey != "" || card.NoteID <= 0 {
		return
	}
	atts, err := kb.Attachments(ctx, card.NoteID)
	if err != nil || len(atts) == 0 {
		return
	}
	if key, name := pickImageAttachment(atts, card.ImageName); key != "" {
		card.ImageKey = key
		if card.ImageName == "" {
			card.ImageName = name
		}
	}
}

// attachMaterialImageKeys — то же для материалов: `PhotoKey` заполняется по
// вложению заметки, из которой материал заведён.
//
// Материалы читаются для каждого показа ★ «Материалы» и для документа, поэтому
// обход ограничен теми, у кого есть заметка и ещё нет картинки.
func attachMaterialImageKeys(ctx context.Context, kb KBReader, materials []Material) []Material {
	if kb == nil || len(materials) == 0 {
		return materials
	}
	for i := range materials {
		if materials[i].PhotoKey != "" || materials[i].NoteID <= 0 {
			continue
		}
		atts, err := kb.Attachments(ctx, materials[i].NoteID)
		if err != nil || len(atts) == 0 {
			continue
		}
		if key, _ := pickImageAttachment(atts, ""); key != "" {
			materials[i].PhotoKey = key
		}
	}
	return materials
}

// offerCardsWithImages — карточки ВЫБРАННЫХ систем предложения с картинками.
//
// Кеш систем (`offers_context`) хранит карточки, прочитанные ДО появления
// `ImageKey` (2026-11-05): у них есть `imageName`, а ключ пуст. Экспорт берёт
// карточки из кеша, поэтому без этого шага метка `{{система}}` печатала систему
// без картинки — живая жалоба 2026-11-07. Привязка идёт только по выбранным
// карточкам: их одна-две, а не все 376 систем.
//
// Ошибка Базы знаний карточку не теряет: картинка необязательна.
func offerCardsWithImages(ctx context.Context, kb KBReader, cards []SystemCard, offer Offer) []SystemCard {
	return attachImageKeys(ctx, kb, cardsForOffer(cards, offer))
}

// kbReaderOrNil — клиент Базы знаний как интерфейс, либо nil.
//
// Отдельная функция, потому что типизированный nil в интерфейсе НЕ равен nil:
// `attachImageKeys` проверил бы `kb == nil`, получил false и упал на первом
// вызове. Проверка «настроен ли» живёт в одном месте.
func kbReaderOrNil(kb *KBClient) KBReader {
	if kb == nil || !kb.configured() {
		return nil
	}
	return kb
}

// pickImageAttachment выбирает картинку из вложений карточки.
//
// Порядок: точное совпадение имени (без учёта регистра), затем совпадение без
// расширения и с «нормализованным» именем (Obsidian хранит имя файла ровно как
// в vault, а в `![[…]]` бывает без `.png`), затем первая картинка.
func pickImageAttachment(atts []KBAttachment, wantName string) (key, name string) {
	want := strings.ToLower(strings.TrimSpace(wantName))
	if want != "" {
		if att, ok := firstImage(atts, func(att KBAttachment) bool {
			return strings.ToLower(strings.TrimSpace(att.Filename)) == want
		}); ok {
			return att.Key, att.Filename
		}
		// Второй проход: сравнение без расширения — `![[картинка.png|335]]` и
		// вложение `картинка` (или наоборот).
		wantStem := stripImageExt(want)
		if att, ok := firstImage(atts, func(att KBAttachment) bool {
			return stripImageExt(att.Filename) == wantStem
		}); ok {
			return att.Key, att.Filename
		}
	}
	// Третий: первая картинка вообще. Карточка продукта почти всегда несёт
	// одну; пустая картинка там, где она нарисована в заметке, читается как
	// «система без картинки».
	if att, ok := firstImage(atts, func(KBAttachment) bool { return true }); ok {
		return att.Key, att.Filename
	}
	return "", ""
}

// firstImage — первая картинка с ключом, подходящая под условие.
func firstImage(atts []KBAttachment, match func(KBAttachment) bool) (KBAttachment, bool) {
	for _, att := range atts {
		if att.Key != "" && isImageMime(att.Mime, att.Filename) && match(att) {
			return att, true
		}
	}
	return KBAttachment{}, false
}

// isImageMime — вложение является картинкой, которую умеет вставить Word.
func isImageMime(mime, filename string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(mime, "image/"):
		return true
	case mime != "":
		return false
	}
	switch strings.ToLower(extOf(filename)) {
	case "png", "jpg", "jpeg", "gif", "bmp":
		return true
	}
	return false
}

// stripImageExt — имя файла без расширения картинки.
//
// Через это переходит и цель из заметки, и имя вложения: сравнение «с
// расширением» и «без» должно идти по одной и той же нормальной форме, иначе
// `Схема` (из `![[Схема]]`) не найдёт `Схема.png` — поймано тестом.
func stripImageExt(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch ext := extOf(name); ext {
	case "png", "jpg", "jpeg", "gif", "bmp", "webp", "svg":
		return strings.TrimSuffix(name, "."+ext)
	}
	return name
}
