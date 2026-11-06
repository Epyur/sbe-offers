package main

import (
	"context"
	"strings"
)

// Картинки систем и материалов в блоках формы (2026-11-05).
//
// Требование пользователя: «нужно сразу проработать механизм вставки
// изображений, как сформировать инструкцию, что в конкретном блоке текста
// должна быть вставлена картинка системы и/или материала».
//
// Ради этого узел `image` получил источник картинки — `imageOf`:
//
//	system    — картинка ВЫБРАННОЙ системы КП (при нескольких системах —
//	            по номеру: 1, 2, … в порядке выбора);
//	material  — картинка материала из позиций КП (по номеру позиции);
//	static    — ключ вложения, выбранный руками (как было до 2026-11-05).
//
// Почему источник, а не готовый ключ: форма общая для всех предложений. В форме
// не может лежать «картинка системы 34» — система выбирается в каждом КП своя.
// Источник говорит, ЧТО взять, а конкретную карточку даёт само предложение.
//
// Отдельным узлом, а не картинкой внутри абзаца: Word не умеет ставить картинку
// посреди run, и попытка «в тексте» смешала бы плавающую картинку с потоком
// текста так, как человек не задумывал.

// Источники картинки узла `image`.
const (
	imageOfStatic   = ""
	imageOfSystem   = "system"
	imageOfMaterial = "material"
)

// imageOf нормализует источник из JSON.
func imageOf(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case imageOfSystem, "система":
		return imageOfSystem
	case imageOfMaterial, "материал":
		return imageOfMaterial
	default:
		return imageOfStatic
	}
}

// resolvedImage — что вставлять: ключ, имя файла и подпись.
type resolvedImage struct {
	Key   string
	Name  string
	Title string
}

// resolveNodeImage — какая картинка должна стоять в узле.
//
// Возвращает false, если картинки нет — узел молчит, как и узел `prompt_text`
// при недоступной модели. Форма, сохранённая под другую систему, не должна
// останавливать сборку документа.
func resolveNodeImage(node blockNode, rc renderCtx) (resolvedImage, bool) {
	switch imageOf(node.ImageOf) {
	case imageOfSystem:
		return resolveSystemImage(node, rc)
	case imageOfMaterial:
		return resolveMaterialImage(node, rc)
	default:
		if strings.TrimSpace(node.ImageKey) == "" {
			return resolvedImage{}, false
		}
		return resolvedImage{Key: node.ImageKey, Name: node.ImageName, Title: ""}, true
	}
}

// resolveSystemImage — картинка выбранной системы по её номеру в предложении.
func resolveSystemImage(node blockNode, rc renderCtx) (resolvedImage, bool) {
	cards := cardsForOffer(rc.cards, rc.offer)
	if len(cards) == 0 {
		return resolvedImage{}, false
	}
	card := cards[clampIndex(node.SystemIndex, len(cards))]
	if card.ImageKey == "" {
		return resolvedImage{}, false
	}
	return resolvedImage{Key: card.ImageKey, Name: card.ImageName, Title: card.Title}, true
}

// resolveMaterialImage — картинка материала из позиции предложения.
func resolveMaterialImage(node blockNode, rc renderCtx) (resolvedImage, bool) {
	if len(rc.offer.Items) == 0 {
		return resolvedImage{}, false
	}
	item := rc.offer.Items[clampIndex(node.ItemIndex, len(rc.offer.Items))]
	if strings.TrimSpace(item.PhotoKey) == "" {
		return resolvedImage{}, false
	}
	return resolvedImage{Key: item.PhotoKey, Name: item.ImageName, Title: item.Name}, true
}

// clampIndex — номер из формы в индексы слайса (0-based).
//
// Номер не задан или вышел за список — берём первый: так форма, сохранённая
// под прошлый КП, всё равно печатает картинку вместо того, чтобы промолчать.
func clampIndex(n, length int) int {
	if n <= 0 || n > length {
		return 0
	}
	return n - 1
}

// showCardImages — узлы формы рисуют картинку системы/материала.
//
// На входе — уже разрешённые ключи (см. attachImageKeys и
// attachMaterialImageKeys): узел ничего не знает про Базу знаний.
func showCardImages(ctx context.Context, node blockNode, media *docxMedia, images ImageSource, rc renderCtx) (string, bool) {
	img, ok := resolveNodeImage(node, rc)
	if !ok {
		return "", false
	}
	// Подпись: своя из узла, иначе название системы/материала. Пустое название
	// лучше выдуманного.
	caption := strings.TrimSpace(node.Caption)
	if caption == "" {
		caption = img.Title
	}
	node.ImageKey = img.Key
	node.ImageName = img.Name
	node.Caption = caption
	return imageNodeXML(node, media, images)
}
