package main

// Вид блока `diagram`: mermaid → PNG в DOCX (2026-11-17, шаг 5 плана
// docs/superpowers/plans/2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md).
//
// Решение спеки (§7.2): в этом заходе схемы только `mermaid`. Произвольный
// SVG не поддерживается намеренно — `agent-mermaid` рендерит mermaid-код, а
// не произвольный `<svg>`; его растеризация была бы отдельной фичей
// agent-service. Поэтому `lang:"svg"` и любой другой язык — блок ВЫБРАСЫВАЕТСЯ
// с записью в журнал, а не печатается каким-то иным путём (не «молча заменим»).
//
// Рендер идёт через agent-service (`POST /internal/agent/render`): mermaid и
// chromium живут там, и тащить их в offers-service нельзя (спека §3.1, §4.2).
// Ответ — PNG в base64 с размерами пикселей; размеры превращаются в EMU так,
// чтобы ширина была по тексту (≤16 см), а высота — по пропорции PNG.
//
// Ошибка рендера — ВИДИМАЯ строка «[Схема не отрисована: <причина>]» плюс код
// схемы (§9.2). Молчащее пустое место менеджер читает как недоделанный
// документ, а причина отказа (нет секрета, сервис молчит, кривой mermaid)
// остаётся в журнале сборки.

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"log"
	"strings"
	"sync"
)

// sectionDiagramCounter — сквозной счётчик подписей «Схема N» в документе.
//
// Один на всю печать (layoutContext.sectionNumbering), потому что подписи идут
// номерами по документу, а не с единицы в каждом подразделе. Счётчик общий
// указатель и пишется из горутин (подразделы параллельны) — поэтому под замком.
type sectionDiagramCounter struct {
	mu sync.Mutex
	n  int
}

// next выдаёт следующий номер схемы (1, 2, …).
func (c *sectionDiagramCounter) next() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return c.n
}

// sectionDiagramRendered — результат растеризации одной схемы для печати.
type sectionDiagramRendered struct {
	// PNG — байты картинки; nil — рендер не удался (Reason заполнена).
	PNG    []byte
	Width  int
	Height int
	Reason string
}

// sectionDiagramRender — как получить PNG по mermaid-коду. Переменная, а не
// прямой вызов клиента, чтобы тесты подставляли рендер без сети (тот же приём,
// что у вставки картинок: тест не должен ходить в хранилище).
var sectionDiagramRender = renderDiagramWithAgent

// renderDiagramWithAgent — настоящий рендер через agent-service.
func renderDiagramWithAgent(code string) sectionDiagramRendered {
	client := NewAgentServiceClient()
	if !client.configured() {
		return sectionDiagramRendered{Reason: "рендер схем не настроен (agent-service не подключён)"}
	}
	out, err := client.Render(context.Background(), "mermaid", code)
	if err != nil {
		return sectionDiagramRendered{Reason: err.Error()}
	}
	if !strings.EqualFold(strings.TrimSpace(out.Format), "png") {
		return sectionDiagramRendered{Reason: "рендерер вернул не PNG (" + out.Format + ")"}
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(out.Base64))
	if err != nil || len(data) == 0 {
		return sectionDiagramRendered{Reason: "рендерер вернул пустую или битую картинку"}
	}
	width, height := out.Width, out.Height
	if width <= 0 || height <= 0 {
		// Размеров в ответе нет — читаем их из самого PNG, чтобы не врать
		// пропорцией по умолчанию: у mermaid хватает широких схем.
		if cfg, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
			width, height = cfg.Width, cfg.Height
		}
	}
	if width <= 0 || height <= 0 {
		return sectionDiagramRendered{Reason: "не удалось определить размеры схемы"}
	}
	return sectionDiagramRendered{PNG: data, Width: width, Height: height}
}

// sectionDiagramXML — печать блока `diagram`: картинка в строку абзаца и
// подпись «Схема N — …» отдельным абзацем по центру.
func sectionDiagramXML(block sectionBlock, lc layoutContext) string {
	code := strings.TrimSpace(block.Code)
	if code == "" {
		return ""
	}
	// Язык: пустой принимаем как mermaid (модель иногда опускает поле), всё
	// остальное — выбрасываем (§7.2). Запись в журнал обязательна: иначе
	// «схема не вышла» будет искать вёрстку, а не язык блока.
	lang := strings.ToLower(strings.TrimSpace(block.Lang))
	if lang == "" {
		lang = "mermaid"
	}
	if lang != "mermaid" {
		log.Printf("раздел: схема с lang=%q не печатается (поддерживается только mermaid)", block.Lang)
		return ""
	}
	counter := lc.sectionNumbering
	if counter == nil {
		counter = &sectionDiagramCounter{}
	}
	number := counter.next()
	if lc.layout == nil || lc.layout.media == nil {
		// Печать вне пакета (редкий путь сборки без макета): видимая строка, а
		// не пустота — схема была запрошена моделью.
		return sectionDiagramFallback(number, block.Caption, "схема некуда вставить (нет сборщика документа)")
	}
	rendered := sectionDiagramRender(code)
	if rendered.PNG == nil {
		reason := strings.TrimSpace(rendered.Reason)
		if reason == "" {
			reason = "рендер не удался"
		}
		log.Printf("раздел: схема %d не отрисована: %s", number, reason)
		if lc.report != nil {
			lc.report.addFailed()
		}
		return sectionDiagramFallback(number, block.Caption, reason)
	}
	cxa, cya, readable := sectionDiagramExtent(rendered.Width, rendered.Height)
	if !readable {
		// Схема вышла нечитаемо вытянутой (2026-11-17, живой дефект 016/2026:
		// 4271×132 → при 16 см ширины высота ~0,5 см, «полоска»). Вставлять
		// такую полоску в документ — обманывать менеджера: он видит «схема
		// есть», а прочесть нельзя. Печатаем видимую подсказку и сам код — что
		// просить, чтобы схема стала вертикальной.
		log.Printf("раздел: схема %d слишком вытянута (%dx%d) — печатаю подсказку вместо полоски",
			number, rendered.Width, rendered.Height)
		if lc.report != nil {
			lc.report.addFailed()
		}
		return sectionDiagramUnreadable(number, block.Caption, rendered.Width, rendered.Height)
	}
	drawing := lc.layout.media.add(rendered.PNG, "png", "image/png", cxa, cya)
	if drawing == "" {
		return sectionDiagramFallback(number, block.Caption, "картинку не удалось вставить в документ")
	}
	var sb strings.Builder
	// wp:inline (не anchor): схема не должна обтекаться текстом — спека §9.2
	// требует inline, иначе Word 2007 «плывёт» вокруг неё.
	sb.WriteString(imageParagraphXML(drawing))
	sb.WriteString(paragraphXMLOpts(sectionDiagramCaption(number, block.Caption), "center", paraOpts{NoIndent: true}))
	if lc.report != nil {
		lc.report.addGenerated()
	}
	return sb.String()
}

// sectionDiagramExtent — размеры схемы в EMU: ширина по тексту (≤16 см), высота
// по пропорции PNG; слишком высокая — масштабируется по высоте листа (≤20 см).
//
// 1 см = 360000 EMU. Пропорция сохраняется всегда — схема не растягивается.
//
// Третий результат — «читаемо ли». Если схема вышла полоской (ширина/высота
// больше `maxDiagramAspect`), то при 16 см ширины высота получается в
// миллиметры, и рисовать её бессмысленно: печать заменит её видимой подсказкой
// (живой дефект 016/2026, 2026-11-17: 4271×132 → полоска 16 см × 0,5 см).
const maxDiagramAspect = 5

func sectionDiagramExtent(widthPx, heightPx int) (int64, int64, bool) {
	const (
		emuPerCm    = 360000
		maxWidthCm  = 16
		maxHeightCm = 20
	)
	if widthPx <= 0 || heightPx <= 0 {
		widthPx, heightPx = 4, 3
	}
	// Полоска: ширина во столько раз больше высоты, что на странице это
	// нечитаемая линия. Читаемость — часть контракта схемы, не косметика.
	// Сравнение «в ширинах» (без деления), чтобы порог был точным.
	if heightPx <= 0 || widthPx > maxDiagramAspect*heightPx {
		return 0, 0, false
	}
	cx := int64(maxWidthCm * emuPerCm)
	cy := cx * int64(heightPx) / int64(widthPx)
	maxCy := int64(maxHeightCm * emuPerCm)
	if cy > maxCy {
		// Слишком высокая: ужимаем по высоте, ширину считаем по той же
		// пропорции (cx = cy * w / h).
		cy = maxCy
		cx = cy * int64(widthPx) / int64(heightPx)
	}
	if cx < 1 {
		cx = 1
	}
	return cx, cy, true
}

// sectionDiagramCaption — подпись под схемой: «Схема N — …».
//
// Номер ставит СЕРВЕР, а не модель (§9.2): модель пишет подпись без номера
// (контракт просит «Схема 1 — …», но модель иногда повторяет номер в тексте);
// чтобы не было «Схема 3 — Схема 3 …», ведущее «Схема N» из подписи снимается.
func sectionDiagramCaption(number int, caption string) string {
	text := strings.TrimSpace(caption)
	text = stripDiagramCaptionPrefix(text)
	prefix := "Схема " + itoa(number)
	if text == "" {
		return prefix
	}
	return prefix + " — " + text
}

// stripDiagramCaptionPrefix снимает ведущее «Схема N —»/«Схема N:» из подписи.
//
// Разбор ручной, а не регуляркой: подпись приходит от модели и не должна
// зависеть от того, что `\b` не работает на кириллице (правило AGENTS.md) —
// здесь и кириллица, и цифры.
func stripDiagramCaptionPrefix(text string) string {
	lower := strings.ToLower(text)
	if !strings.HasPrefix(lower, "схема") {
		return text
	}
	rest := strings.TrimLeft(text[len("схема"):], " \t")
	// Ведём ведущие цифры.
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return text
	}
	rest = strings.TrimLeft(rest[digits:], " \t")
	// Разделитель «—», «–», «-», «:» или «.» — иначе это не наш префикс.
	// Сравнение по строке, а не по байту: «—» в UTF-8 — три байта, и `rest[0]`
	// дал бы половину символа (живой отказ компилятора).
	if rest == "" {
		return ""
	}
	for _, sep := range []string{"—", "–", "-", ":", "."} {
		if strings.HasPrefix(rest, sep) {
			return strings.TrimLeft(rest[len(sep):], " \t")
		}
	}
	return text
}

// sectionDiagramFallback — видимый отказ вместо молчания (§9.2).
//
// Печатается подпись «Схема N — …» (если была), строка «[Схема не отрисована:
// <причина>]» и сам mermaid-код моноширинным блоком: менеджер видит, что схема
// задумана, а разработчик — почему не вышло. Иначе пустое место читается как
// недоделанный документ.
func sectionDiagramFallback(number int, caption, reason string) string {
	var sb strings.Builder
	sb.WriteString(paragraphXMLOpts(sectionDiagramCaption(number, caption), "center", paraOpts{NoIndent: true}))
	sb.WriteString(sectionParagraphXML("[Схема не отрисована: " + strings.TrimSpace(reason) + "]"))
	return sb.String()
}

// sectionDiagramUnreadable — схема получилась нечитаемо вытянутой (2026-11-17).
//
// Печатаем подпись «Схема N — …», честную строку «Схема получилась слишком
// вытянутой …» и подсказку, что поправить в инструкции раздела (меньше узлов,
// короткие подписи, вертикальная схема). Полоску в 16×0,5 см не вставляем: она
// занимает место, но её нельзя прочесть — это хуже честного сообщения.
func sectionDiagramUnreadable(number int, caption string, widthPx, heightPx int) string {
	var sb strings.Builder
	sb.WriteString(paragraphXMLOpts(sectionDiagramCaption(number, caption), "center", paraOpts{NoIndent: true}))
	sb.WriteString(sectionParagraphXML(
		"[Схема получилась слишком вытянутой (" + itoa(widthPx) + "×" + itoa(heightPx) +
			" px) — на странице она стала бы нечитаемой полоской, поэтому не напечатана. " +
			"Упростите схему: меньше узлов, короткие подписи, вертикальная раскладка (flowchart TD с 3–5 узлами).]"))
	return sb.String()
}
