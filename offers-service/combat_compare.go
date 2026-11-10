package main

import (
	"context"
	"log"
	"strings"
)

// Сравнение боевого листка (2026-11-11).
//
// Решение пользователя: содержание боевого листка всегда одинаковое — сначала
// перечень продуктов, затем их сравнение. Настраивать форму незачем; единственная
// настройка — УТОЧНЕНИЕ менеджера: на чём он хочет, чтобы разбор сделал акцент
// («огонестойкость», «срок службы», «стоимость владения»).
//
// Сравнение пишет модель общим ключом llm-service (как у КП). Оно строится в двух
// частях:
//
//	1. общее сравнение — все продукты стороной «наше против конкурентов»;
//	2. по каждому продукту отдельно — чем он отличается от нашего и что спросить
//	   у заказчика.
//
// Почему модель, а не жёсткая таблица: у продуктов конкурентов карточки свободной
// формы, и наложение характеристик построчно дало бы таблицу с пустыми клетками
// там, где названия строк не совпадают («Теплопроводность, λА» и «λ10» — одна
// величина, разные записи). Модель сводит их в текст, а цифры берёт из карточек.

// combatCompareSystem — роль модели. Просьба писать обычным рабочим русским и не
// выдумывать цифр: карточки конкурентов — единственный источник фактов, и
// придуманное число здесь дороже пропущенного.
const combatCompareSystem = `Ты аналитик коммерческой службы и готовишь короткую справку менеджеру
для спора с заказчиком. Тебе дают карточки продуктов: наш продукт и продукты
конкурентов. Напиши сравнение простым рабочим русским языком, без рекламных
оборотов и без markdown.

Правила:
- Факты (характеристики, цифры, классы, ГОСТы) бери ТОЛЬКО из карточек.
  Ничего не выдумывай: если по пункту данных нет — честно скажи «в карточке
  не указано» и предложи, что уточнить.
- Не называй продукты «лучшими»; объясняй, чем они отличаются и в каких
  условиях какой выигрывает.
- Не сравнивай по цене: цен в карточках нет.
- Каждый абзац — 2-4 предложения.

Структура ответа (ровно эти заголовки, дословно):
## Общее сравнение
(2-4 абзаца: чем наш продукт отличается от продуктов конкурентов в целом)
## По каждому продукту
### <название продукта конкурента>
(1-2 абзаца: в чём сильная и слабая сторона против нашего, что спросить у
заказчика; для каждого продукта конкурента по порядку)`

// buildCombatComparePrompt собирает запрос модели: карточки обеих сторон плюс
// уточнение менеджера.
//
// Карточки идут текстом (`PromptText`), а не разметкой: модель не рисует
// документ, ей нужны факты. Уточнение — отдельной строкой вверху, чтобы модели
// было ясно, на что смотреть, но не заменяло структуру ответа.
func buildCombatComparePrompt(hint string, ourCards, competitorCards []SystemCard) string {
	var b strings.Builder
	if h := strings.TrimSpace(hint); h != "" {
		b.WriteString("На что обратить особое внимание (уточнение менеджера): ")
		b.WriteString(h)
		b.WriteString("\n\n")
	}
	b.WriteString("НАШ ПРОДУКТ:\n")
	for _, card := range ourCards {
		b.WriteString("- ")
		b.WriteString(titleOr(card.Title))
		b.WriteString("\n")
		writeCardFacts(&b, card)
	}
	b.WriteString("\nПРОДУКТЫ КОНКУРЕНТОВ:\n")
	for _, card := range competitorCards {
		b.WriteString("- ")
		b.WriteString(titleOr(card.Title))
		b.WriteString("\n")
		writeCardFacts(&b, card)
	}
	if len(competitorCards) == 0 {
		b.WriteString("(продукты конкурентов в листок не добавлены)\n")
	}
	return b.String()
}

// writeCardFacts пишет факты карточки для модели: описание, состав, ключевые
// характеристики. Таблица характеристик режется: в живой карточке конкурента
// сорок строк, и всю таблицу модель либо не дочитает, либо утонет в ней.
func writeCardFacts(b *strings.Builder, card SystemCard) {
	if desc := strings.TrimSpace(card.Description); desc != "" {
		b.WriteString("  описание: ")
		b.WriteString(trimForPrompt(desc, 1200))
		b.WriteString("\n")
	}
	if len(card.Composition) > 0 {
		b.WriteString("  состав: ")
		b.WriteString(trimForPrompt(strings.Join(card.Composition, "; "), 800))
		b.WriteString("\n")
	}
	for i, row := range card.Specifications {
		// Первая строка — шапка таблицы, её модели не нужно.
		if i == 0 || i > combatSpecRows {
			continue
		}
		line := strings.Join(row, " — ")
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString("  характеристики: ")
		b.WriteString(trimForPrompt(line, 240))
		b.WriteString("\n")
	}
}

// combatSpecRows — сколько строк характеристик отдавать модели. Достаточно,
// чтобы увидеть класс продукта, и мало, чтобы не тратить окно модели на
// размеры и толщины.
const combatSpecRows = 15

// trimForPrompt режет текст по рунам, не разрывая слово.
func trimForPrompt(s string, limit int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	cut := string(runes[:limit])
	if idx := strings.LastIndex(cut, " "); idx > limit/2 {
		cut = cut[:idx]
	}
	return strings.TrimSpace(cut) + "…"
}

// renderCombatComparison печатает раздел сравнения.
//
// Модель недоступна — раздел не пустует: печатается перечень карточек с
// характеристиками (то, что есть в данных). Это честнее пустого места с
// заголовком «Сравнение», которое читается как ошибка печати.
func renderCombatComparison(rc renderCtx, sheet CombatSheet, ourCards, competitorCards []SystemCard) string {
	var sb strings.Builder
	sb.WriteString(headingXML("Сравнение", 2))

	hint := combatHint(sheet)
	text := ""
	if len(competitorCards) > 0 {
		promptText, err := Complete(context.Background(), combatCompareSystem,
			buildCombatComparePrompt(hint, ourCards, competitorCards), 0)
		if err != nil {
			log.Printf("боевой листок %s: сравнение не написано моделью: %v", sheet.Number, err)
		} else {
			text = normalizeBodyTypography(strings.TrimSpace(promptText))
		}
	}
	if text != "" {
		sb.WriteString(combatComparisonMarkup(text))
	} else {
		// Запасной вид: карточки с характеристиками. Так же, как раньше, но под
		// заголовком сравнения и без обещания, что это разбор.
		writeCombatCards(&sb, "Наш продукт", ourCards, rc.images, rc.media)
		writeCombatCards(&sb, "Продукты конкурентов", competitorCards, rc.images, rc.media)
	}
	return sb.String()
}

// combatHint — уточнение менеджера из листка.
//
// Хранится в `form_values["hint"]`: отдельной колонки нет, а в листке поле
// формы осталось ровно одно — это. Пусто — сравнение без акцента.
func combatHint(sheet CombatSheet) string {
	if sheet.FormValues == nil {
		return ""
	}
	return strings.TrimSpace(sheet.FormValues[combatHintField])
}

// combatHintField — ключ уточнения в form_values листка.
const combatHintField = "hint"

// combatComparisonMarkup печатает текст сравнения.
//
// Модель отвечает по структуре с заголовками «## …» и «### …». Заголовки
// печатаются своими уровнями, остальное — абзацами: тот же приём, что у текста
// по промту в форме КП.
func combatComparisonMarkup(text string) string {
	var sb strings.Builder
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "### "):
			sb.WriteString(headingXML(strings.TrimSpace(strings.TrimPrefix(trimmed, "### ")), 4))
		case strings.HasPrefix(trimmed, "## "):
			sb.WriteString(headingXML(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")), 3))
		case strings.HasPrefix(trimmed, "# "):
			// Одинокая «#» моделью не задумана, но и терять строку незачем.
			sb.WriteString(headingXML(strings.TrimSpace(strings.TrimPrefix(trimmed, "# ")), 3))
		case trimmed == "":
			continue
		default:
			sb.WriteString(paragraphsXML(trimmed, "", paraOpts{}))
		}
	}
	return sb.String()
}
