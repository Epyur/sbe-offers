package main

import (
	"strings"

	"mailers/offers/skills"
)

// docxFileNameFor собирает имя файла «<заголовок> <номер>.docx».
//
// Отдельно от docxFileName (тот заточен под КП с клиентом): у боевого листка
// клиента нет — есть название и номер. Косая черта номера меняется на дефис:
// иначе она создаёт папку при сохранении файла.
func docxFileNameFor(title, number string) string {
	name := strings.TrimSpace(title)
	if name == "" {
		name = "Боевой листок"
	}
	if num := strings.TrimSpace(number); num != "" {
		name += " " + strings.ReplaceAll(num, "/", "-")
	}
	return name + ".docx"
}

// Сборка DOCX боевого листка (2026-11-08, вторая партия 0.8.3).
//
// Боевой листок — тот же документ на фирменном бланке, что и КП, но другого
// назначения: справка-сравнение, а не письмо заказчику. Поэтому печать идёт по
// форме боевого листка (mode=combat) и НЕ печатает то, чего у листка нет:
// таблицу позиций с ценами, итог, тексты секций КП, сводку брифа. Вместо этого
// после блоков печатаются две группы карточек Базы знаний — наш продукт и
// продукты конкурентов, — каждая с описанием, составом, характеристиками,
// картинкой и ссылками.
//
// Переиспользуется весь конвейер текста и печати блоков (`renderBlockContentCtx`
// и `renderCardSectionsWithMedia`): второй рендер разошёлся бы с первым в
// подстановках и разметке. Отличие — только в сборке скелета документа.
//
// `ourCards`/`competitorCards` приходят уже разрешёнными (по noteId из кеша
// систем и конкурентов). Пустые списки допустимы: листок собирается и без
// одной из сторон.
func renderCombatDocx(form OfferForm, sheet CombatSheet, ourCards, competitorCards []SystemCard, images ImageSource) ([]byte, error) {
	// Синтетическое «предложение» — только чтобы конвейер блоков увидел значения
	// служебных полей (номер, дата, менеджер) и связал их с карточками. Позиций
	// нет намеренно: их у боевого листка нет.
	offer := combatAsOffer(sheet)

	// Ссылки — из обеих сторон сравнения: страница продукта и техдокументация
	// нужны менеджеру независимо от того, чей это продукт. Объединение списков
	// может дать дубль URL — убираем повторы по адресу.
	refs := dedupCardRefs(append(collectOfferRefs(ourCards), collectOfferRefs(competitorCards)...))

	media := newTemplateMedia()
	rc := renderCtx{
		values:    offer.FormValues,
		service:   serviceValuesWithCards(offer, ourCards),
		offer:     offer,
		refs:      refs,
		cards:     ourCards,
		images:    images,
		media:     media,
		numbering: newNumberingPlan(numberingAbstractDecimal),
	}

	var sb strings.Builder
	// Титула-бланка нет: боевой листок печатают по своей форме, где титул —
	// обычный блок. Пустая форма — соберём хотя бы заголовок, чтобы файл не был
	// пустым (та же политика, что у КП).
	blocks := visibleBlocks(form)
	if len(blocks) == 0 {
		sb.WriteString(headingXML(titleOr(form.Title), 1))
	}
	for _, block := range blocks {
		content, err := renderBlockContentCtx(block, rc)
		if err != nil {
			return nil, err
		}
		sb.WriteString(content)
	}

	// Наш продукт и продукты конкурентов — двумя разделами. Печатаем всегда,
	// даже если администратор не завёл под них блок в форме: сравнение без
	// одной из сторон — это не боевой листок.
	writeCombatCards(&sb, "Наш продукт", ourCards, images, media)
	writeCombatCards(&sb, "Продукты конкурентов", competitorCards, images, media)

	body, linkRels := applyHyperlinks(sb.String(), refs)
	if _, relErr := relsXMLWithLinks(nil, linkRels); relErr != nil {
		body = sb.String()
		linkRels = nil
	}
	return buildDocxWithNumbering(body, media, linkRels, rc.numbering)
}

// dedupCardRefs убирает ссылки с повторяющимся адресом. Обе стороны сравнения
// могут ссылаться на один и тот же документ (техлист у всех один) — в списке
// ссылок он должен стоять один раз.
func dedupCardRefs(refs []skills.CardRef) []skills.CardRef {
	seen := make(map[string]bool, len(refs))
	out := make([]skills.CardRef, 0, len(refs))
	for _, ref := range refs {
		if ref.URL == "" || seen[ref.URL] {
			continue
		}
		seen[ref.URL] = true
		out = append(out, ref)
	}
	return out
}

// combatAsOffer — боевой листок в виде, который понимает конвейер блоков.
//
// Всё, что не относится к листку (позиции, клиент, итог), пусто: рендер блоков
// этих полей не печатает, а служебные значения (номер, дата, менеджер) берутся
// из листка.
func combatAsOffer(sheet CombatSheet) Offer {
	values := sheet.FormValues
	if values == nil {
		values = map[string]string{}
	}
	return Offer{
		ID:         sheet.ID,
		Number:     sheet.Number,
		OwnerEmail: sheet.OwnerEmail,
		FormValues: values,
		CreatedAt:  sheet.CreatedAt,
		Status:     sheet.Status,
	}
}

// writeCombatCards печатает группу карточек под заголовком раздела.
//
// Пустая группа не печатается вовсе: заголовок без карточек («Продукты
// конкурентов» и пусто под ним) читался бы как недоделанный документ.
func writeCombatCards(sb *strings.Builder, title string, cards []SystemCard, images ImageSource, media *docxMedia) {
	if len(cards) == 0 {
		return
	}
	sb.WriteString(headingXML(title, 2))
	for _, card := range cards {
		writeCombatCard(sb, card, images, media)
	}
}

// writeCombatCard печатает одну карточку: название, описание, картинку, состав,
// характеристики.
//
// Не переиспользует `renderCardSectionsWithMedia` намеренно: тот печатает
// заголовок «Система: …», а у конкурента это не система, и менять общий
// печататель ради подписи значило бы трогать печать КП. Здесь тот же конвейер
// текста (`paragraphsXML`, `writeCardImage`, `tableXML`), только своя подпись.
func writeCombatCard(sb *strings.Builder, card SystemCard, images ImageSource, media *docxMedia) {
	filled := strings.TrimSpace(card.Description) != "" || len(card.Composition) > 0 ||
		len(card.Specifications) > 1 || card.ImageKey != ""
	// Карточка без содержания не печатается: заголовок с пустым телом
	// читался бы как пропуск в документе.
	if !filled {
		return
	}
	sb.WriteString(headingXML(card.Title, 3))
	if strings.TrimSpace(card.Description) != "" {
		sb.WriteString(paragraphsXML(card.Description, "", paraOpts{}))
	}
	writeCardImage(sb, card, images, media)
	if len(card.Composition) > 0 {
		sb.WriteString(headingXML("Состав", 4))
		for _, item := range card.Composition {
			sb.WriteString(listItemXML(item))
		}
	}
	if len(card.Specifications) > 1 {
		sb.WriteString(headingXML("Характеристики", 4))
		sb.WriteString(tableXML(card.Specifications))
	}
}
