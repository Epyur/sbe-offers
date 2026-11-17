package main

import (
	"context"
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
// renderCombatDocx собирает DOCX боевого листка.
//
// Содержание боевого листка (2026-11-16, шаг 2a плана
// 2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md): заголовок,
// перечень продуктов, затем СОДЕРЖАТЕЛЬНЫЕ РАЗДЕЛЫ формы листка
// (`mode='combat'`) — тот же конвейер, что у КП (`combat_sections.go`).
// Разделов нет или все пусты — прежний вид: общий текст сравнения и карточки.
// Так документ никогда не остаётся без сравнения, а настраиваемость появилась
// без второй печати.
//
// Макет (.docx на фирменном бланке) вставляется меткой `{{сравнение}}`: там, где
// она стоит, печатается весь листок. Без макета — тот же листок печатается
// встроенным оформлением.
func (s *Server) renderCombatDocx(ctx context.Context, layout []byte, sheet CombatSheet,
	ourCards, competitorCards []SystemCard, images ImageSource) ([]byte, error) {
	body, refs, links, media, err := s.buildCombatBody(ctx, sheet, ourCards, competitorCards, images)
	if err != nil {
		return nil, err
	}
	return packCombatDocx(layout, sheet, body, refs, links, media, images)
}

// buildCombatBody собирает XML тела листка и всё, что нужно упаковке:
// ссылки, сборщик и медиа. Отдельно от упаковки — потому что одна и та же
// сборка идёт и в макет (через метку), и во встроенный вид.
func (s *Server) buildCombatBody(ctx context.Context, sheet CombatSheet, ourCards, competitorCards []SystemCard,
	images ImageSource) (string, []skills.CardRef, *layoutRuntime, *docxMedia, error) {
	// Синтетическое «предложение» — только чтобы конвейер увидел служебные поля
	// (номер, дата) и связал их с карточками. Позиций у листка нет.
	offer := combatAsOffer(sheet)

	// Ссылки — ТОЛЬКО наши (2026-11-17, требование пользователя: «ссылки на
	// ресурсы конкурента не приводим»). Листок — документ для заказчика, и чужие
	// страницы/техлисты в его конце читаются как наша рекомендация. Повторы по
	// адресу убираем.
	refs := dedupCardRefs(collectOfferRefs(ourCards))

	media := newTemplateMedia()
	links := &layoutRuntime{media: media, numbering: newNumberingPlan(numberingAbstractDecimal)}
	rc := renderCtx{
		values:        offer.FormValues,
		service:       serviceValuesWithCards(offer, ourCards),
		offer:         offer,
		refs:          refs,
		cards:         ourCards,
		images:        images,
		media:         media,
		layoutRuntime: links,
		numbering:     links.numbering,
	}

	var sb strings.Builder
	sb.WriteString(headingXML("Боевой листок № "+sheet.Number, 1))
	// Перечень продуктов — первым, как просил пользователь: сначала кто с кем
	// сравнивается, потом самое сравнение.
	sb.WriteString(headingXML("Продукты сравнения", 2))
	sb.WriteString(combatProductList(ourCards, competitorCards))
	// Сравнение: сначала разделы формы листка (настраиваемые подразделы), а если
	// их нет или они пусты — прежний вид. Ошибка модели в разделе не гасит
	// документ: перечень продуктов уже есть, и запасной вид тоже.
	//
	// true — разделы напечатаны; false — запасной вид (карточки).
	//
	// Почему пример в конце, а не «вместо»: пример обязан пройти тот же путь,
	// что и разделы, иначе кэш и печать картинок в нём могут разойтись (сборки в
	// двух местах). Заголовок «Сравнение» ставит он же — у разделов своего нет.
	sections, ok := s.renderCombatSections(ctx, sheet, ourCards, competitorCards, refs, images, media, links)
	if ok {
		sb.WriteString(sections)
	} else {
		sb.WriteString(renderCombatComparison(rc, sheet, ourCards, competitorCards))
	}

	return sb.String(), refs, links, media, nil
}

// packCombatDocx упаковывает тело: по макету, если он загружен, иначе — как
// есть.
func packCombatDocx(layout []byte, sheet CombatSheet, body string, refs []skills.CardRef,
	links *layoutRuntime, media *docxMedia, images ImageSource) ([]byte, error) {
	if len(layout) > 0 {
		offer := combatAsOffer(sheet)
		lc := layoutContext{
			values:  offer.FormValues,
			refs:    refs,
			service: serviceValuesWithCards(offer, nil),
			images:  images,
			offer:   offer,
			layout:  links,
			combat:  &combatLayoutData{markup: body},
		}
		return renderLayoutDocx(layout, lc)
	}
	return buildBodyWithLinks(body, refs, links, media)
}

// combatProductList печатает перечень продуктов обеих сторон.
//
// Это ОГЛАВЛЕНИЕ сравнения, а не описание: одно название на строку. Полные
// карточки и разбор идут в сравнении — иначе документ повторял бы себя.
func combatProductList(ourCards, competitorCards []SystemCard) string {
	var sb strings.Builder
	if len(ourCards) > 0 {
		sb.WriteString(headingXML("Наш продукт", 3))
		for _, card := range ourCards {
			sb.WriteString(listItemXML(titleOr(card.Title)))
		}
	}
	if len(competitorCards) > 0 {
		sb.WriteString(headingXML("Продукты конкурентов", 3))
		for _, card := range competitorCards {
			sb.WriteString(listItemXML(titleOr(card.Title)))
		}
	}
	return sb.String()
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
