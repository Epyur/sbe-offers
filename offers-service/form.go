package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Placeholder — поле формы КП, которое администратор заводит в конфигураторе.
type Placeholder struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Block — блок документа формы (абзацы/заголовки/списки/таблицы, разрывы
// страниц, ссылки и текст по промту).
//
// Размещение на странице (2026-11-01): отступы в пунктах (Word меряет абзацный
// интервал в twips, 1 пт = 20; в JSON пишем пункты — админу понятнее),
// «не разрывать между страницами» и «начинать с новой страницы». Абсолютного
// позиционирования нет намеренно: текстовые поля и якоря не переживают правку
// абзаца человеком, а для письма нужны именно отступы, выравнивание и разрыв.
type Block struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Content        json.RawMessage `json:"content"`
	ShowInClient   bool            `json:"showInClient"`
	ShowInInternal bool            `json:"showInInternal"`
	// SpacingBefore / SpacingAfter — отступ до и после блока в пунктах.
	// 0 — как в стиле бланка; отрицательные не допускаются.
	SpacingBefore int `json:"spacingBefore,omitempty"`
	SpacingAfter  int `json:"spacingAfter,omitempty"`
	// KeepTogether — не разрывать абзацы блока между страницами.
	KeepTogether bool `json:"keepTogether,omitempty"`
	// PageBreakBefore — начинать блок с новой страницы.
	PageBreakBefore bool `json:"pageBreakBefore,omitempty"`
	// Repeat / Source — повтор и источник данных (2026-11-06, шаг D плана
	// «генерация без модели»). `{{block:N}}` для блока с repeat:"systems"
	// печатается по разу на выбранную систему, с repeat:"materials" — по
	// позициям предложения. Source:"card" говорит, что данные блока — карточка
	// Базы знаний, а не собственный текст администратора (сейчас используется
	// для подсказки в редакторе; печать его не требует).
	Repeat string `json:"repeat,omitempty"`
	Source string `json:"source,omitempty"`
}

// Виды повтора блока формы.
const (
	blockRepeatSystems   = "systems"
	blockRepeatMaterials = "materials"
)

// RepeatName — имя повтора: id блока.
//
// Метки макета ссылаются на повтор через `{{… name="S"}}`, и имя — это id
// блока: администратор пишет `{{система name="7"}}`, где 7 — id блока с
// repeat:"systems". Одно имя на два места, и держать его в двух полях незачем.
func (b Block) RepeatName() string {
	if strings.TrimSpace(b.Repeat) == "" {
		return ""
	}
	return strings.TrimSpace(b.ID)
}

// OfferForm — конфигурация формы КП.
type OfferForm struct {
	ID             int64         `json:"id"`
	Mode           string        `json:"mode"`
	Title          string        `json:"title"`
	Placeholders   []Placeholder `json:"placeholders"`
	Blocks         []Block       `json:"blocks"`
	RequiredFields []string      `json:"requiredFields"`
	UpdatedAt      string        `json:"updatedAt"`
	// HasLayout/LayoutName — загружен ли макет .docx (2026-11-05). Сами байты
	// в JSON формы не отдаются: клиенту нужно знать, есть ли макет, а не
	// тащить десятки килобайт в каждом ответе `/form`.
	HasLayout  bool   `json:"hasLayout"`
	LayoutName string `json:"layoutName,omitempty"`
	// LayoutFields/LayoutPrompts — какие метки найдены в макете: администратор
	// должен видеть, что система поняла из его документа, ещё до первой сборки.
	LayoutFields  []string `json:"layoutFields,omitempty"`
	LayoutPrompts []string `json:"layoutPrompts,omitempty"`
}

// maxBlockSpacing — предел отступа блока в пунктах. Свыше 200 пт блок
// уезжает с страницы целиком, и это ошибка админа, а не оформление.
const maxBlockSpacing = 200

// validateForm проверяет форму перед сохранением; пустая строка — форма корректна.
func validateForm(f OfferForm) string {
	if strings.TrimSpace(f.Title) == "" {
		return "не заполнен заголовок формы"
	}
	seenBlocks := make(map[string]bool, len(f.Blocks))
	for _, block := range f.Blocks {
		if strings.TrimSpace(block.ID) == "" {
			return "у блока пустой идентификатор"
		}
		if seenBlocks[block.ID] {
			return "повторяющийся идентификатор блока"
		}
		seenBlocks[block.ID] = true
		if block.SpacingBefore < 0 || block.SpacingAfter < 0 {
			return "отступ блока «" + block.Title + "» не может быть отрицательным"
		}
		if block.SpacingBefore > maxBlockSpacing || block.SpacingAfter > maxBlockSpacing {
			return "отступ блока <" + block.Title + "> больше " + strconv.Itoa(maxBlockSpacing) + " пт"
		}
		// Повтор блока (2026-11-06, шаг D): только известные значения. Молча
		// не повторить блок значило бы напечатать его один раз и не сказать.
		switch block.Repeat {
		case "", blockRepeatSystems, blockRepeatMaterials:
		default:
			return "неизвестный повтор блока <" + block.Title + ">: " + block.Repeat
		}
	}
	seenFields := make(map[string]bool, len(f.Placeholders))
	for _, p := range f.Placeholders {
		if strings.TrimSpace(p.ID) == "" {
			return "пустой идентификатор поля"
		}
		if seenFields[p.ID] {
			return "повторяющийся идентификатор поля"
		}
		seenFields[p.ID] = true
	}
	for _, req := range f.RequiredFields {
		if !seenFields[req] {
			return "обязательное поле «" + req + "» не заведено в списке полей"
		}
	}
	return ""
}

// defaultForm — форма по умолчанию: пока администратор ничего не настроил,
// предложение всё равно собирается.
//
// Порядок — как просил пользователь (2026-11-01): титульный лист, разрыв
// страницы, затем поставщик / получатель / объект, свободный блок про систему и
// материалы (текст пишет модель по подсказке), позиции (их печатает сборщик
// документа сам) и ссылки в конце.
//
// Уже сохранённые формы эта функция не трогает: «строки нет в БД» — это и есть
// «форма не настроена», а сохранённая пустая форма остаётся пустой (правило
// 2026-10-27, живой дефект с подменой формы).
func defaultForm(mode string) OfferForm {
	if mode == combatFormMode {
		return combatDefaultForm()
	}
	return OfferForm{
		Mode:  mode,
		Title: "Коммерческое предложение",
		Placeholders: []Placeholder{
			{ID: "supplier_name", Label: "Поставщик"},
			{ID: "supplier_contacts", Label: "Контакты поставщика"},
			{ID: "client_name", Label: "Получатель (организация)"},
			{ID: "client_contact", Label: "Контактное лицо получателя"},
			{ID: "object_name", Label: "Объект"},
			{ID: "object_address", Label: "Адрес объекта"},
			{ID: "valid_until", Label: "Предложение действительно до"},
			{ID: "conditions", Label: "Условия"},
		},
		Blocks: []Block{
			{
				ID:           "cover",
				Title:        "Титульный лист",
				Content:      json.RawMessage(coverBlockJSON),
				ShowInClient: true,
			},
			{
				ID:              "parties",
				Title:           "Поставщик, получатель, объект",
				Content:         json.RawMessage(partiesBlockJSON),
				ShowInClient:    true,
				PageBreakBefore: true,
			},
			{
				ID:            "description",
				Title:         "Система и материалы",
				Content:       json.RawMessage(descriptionBlockJSON),
				ShowInClient:  true,
				SpacingBefore: 12,
			},
			{
				ID:           "links",
				Title:        "Ссылки",
				Content:      json.RawMessage(`[{"type":"links","level":2}]`),
				ShowInClient: true,
			},
		},
		RequiredFields: []string{"client_name"},
	}
}

// Содержимое блоков формы по умолчанию. Текстом, а не собранным в коде JSON:
// эти строки правятся при изменении шаблона, и держать их рядом проще.
const (
	// coverBlockJSON — титульный лист. Абзацы по центру, подписи полей — в
	// одном абзаце с {{полем}}: пустое поле даёт «Кому: » без пустых строк.
	//
	// Номер и дата — служебными полями (2026-11-02): у каждого КП они свои, и
	// набирать их в форме руками значило бы забыть обновить после копирования.
	coverBlockJSON = `[
		{"type":"paragraph","text":"Коммерческое предложение № {{offer_number}} от {{offer_date}}","align":"center"},
		{"type":"paragraph","text":"{{supplier_name}}","align":"center"},
		{"type":"paragraph","text":"Кому: {{client_name}}","align":"center"},
		{"type":"paragraph","text":"{{client_contact}}","align":"center"}
	]`

	// partiesBlockJSON — содержательная часть: кто предлагает, кому и для
	// какого объекта. Таблицей из двух колонок, как в письмах.
	partiesBlockJSON = `[
		{"type":"heading","text":"Об объекте","level":2},
		{"type":"table","rows":[
			["Поставщик","{{supplier_name}}"],
			["Контакты","{{supplier_contacts}}"],
			["Получатель","{{client_name}}"],
			["Контактное лицо","{{client_contact}}"],
			["Объект","{{object_name}}"],
			["Адрес объекта","{{object_address}}"]
		]}
	]`

	// descriptionBlockJSON — свободный блок: текст пишет модель по подсказке,
	// опираясь на объект, системы из карточек и состав предложения.
	descriptionBlockJSON = `[
		{"type":"heading","text":"Предлагаемое решение","level":2},
		{"type":"prompt_text","instruction":"Опиши предлагаемую систему и материалы, входящие в неё: как они работают вместе и что дают на этом объекте. Опирайся только на данные предложения и карточки систем.","maxWords":200}
	]`
)

// combatFormMode — режим формы боевого листка в таблице offers_form.
//
// Форма боевого листка — ТА ЖЕ форма КП (тот же редактор блоков), только под
// другим режимом. Отдельная таблица не заводится: блоки, поля и макет .docx у
// двух документов одинаковой природы, и вторая таблица разошлась бы с первой
// при первой же правке редактора.
const combatFormMode = "combat"

// clientFormMode — режим обычного коммерческого предложения.
const clientFormMode = "client"

// combatDefaultForm — форма боевого листка по умолчанию.
//
// Отличие от КП — в назначении: это справка-сравнение для менеджера, а не
// письмо заказчику. Поэтому в ней титул с названием и датой, блок нашего
// предложения и блок конкурентов, а также свободный блок аргументации. Редактор
// администратора может перестроить всё это, как и форму КП.
func combatDefaultForm() OfferForm {
	// У боевого листка формы как таковой НЕТ (2026-11-11, решение
	// пользователя): содержание всегда одно — перечень продуктов, затем их
	// сравнение. Настройка осталась одна: УТОЧНЕНИЕ менеджера, на чём сделать
	// акцент в разборе. Прежние поля (наша компания, объект, заказчик,
	// менеджер) были невалидны: заказчика у внутренней справки нет.
	return OfferForm{
		Mode:  combatFormMode,
		Title: "Боевой листок",
		Placeholders: []Placeholder{
			{ID: combatHintField, Label: "Уточнение для разбора"},
		},
		// Блоков нет намеренно: печать листка их не читает. Пустой список
		// честно говорит администратору, что конфигуратор здесь ни на что не
		// влияет, — вместо вида, который кажется настраиваемым.
		Blocks:         []Block{},
		RequiredFields: []string{},
	}
}

func (s *Server) seedForms(ctx context.Context) error {
	// Форму КП сеем пустой строкой: она «настроена», админ правит её в
	// конфигураторе, а до первой правки `loadForm` отдаёт форму по умолчанию
	// только когда строки нет. Для боевого листка строку НЕ сеем намеренно:
	// пока админ не сохранил свою форму, `loadForm` отдаёт `combatDefaultForm`,
	// а пустая "настроенная" строка показала бы пустую форму вместо заготовки.
	_, err := s.pool.Exec(ctx,
		`INSERT INTO offers_form (mode) VALUES ('client') ON CONFLICT (mode) DO NOTHING`)
	return err
}

func (s *Server) loadForm(ctx context.Context, mode string) (OfferForm, error) {
	if strings.TrimSpace(mode) == "" {
		mode = clientFormMode
	}
	var form OfferForm
	var placeholders, blocks, required []byte
	var layoutDocx []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, mode, title, placeholders, blocks, required_fields,
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), layout_docx, layout_name
FROM offers_form WHERE mode = $1`, mode).
		Scan(&form.ID, &form.Mode, &form.Title, &placeholders, &blocks, &required, &form.UpdatedAt,
			&layoutDocx, &form.LayoutName)
	if err != nil {
		// Строки нет вовсе — форма не настроена, показываем поля по умолчанию.
		// Пустые массивы в БД — это НАСТРОЕННАЯ пустая форма, и подменять её
		// нельзя: раньше проверка стояла до разбора JSON и сравнивала nil-слайс,
		// поэтому сохранённая форма не доходила ни до редактора, ни до DOCX.
		return defaultForm(mode), nil
	}
	form.Placeholders = []Placeholder{}
	form.Blocks = []Block{}
	form.RequiredFields = []string{}
	if len(placeholders) > 0 {
		if err := json.Unmarshal(placeholders, &form.Placeholders); err != nil {
			return form, err
		}
	}
	if len(blocks) > 0 {
		if err := json.Unmarshal(blocks, &form.Blocks); err != nil {
			return form, err
		}
	}
	if len(required) > 0 {
		if err := json.Unmarshal(required, &form.RequiredFields); err != nil {
			return form, err
		}
	}
	form.HasLayout = len(layoutDocx) > 0
	form.LayoutFields, form.LayoutPrompts = describeLayout(layoutDocx)
	return form, nil
}

// describeLayout — какие метки есть в макете, для показа администратору.
//
// Разбор терпимый: испорченный макет не должен ломать загрузку формы — иначе
// один неудачный файл скрыл бы от человека весь конфигуратор.
func describeLayout(layout []byte) (fields, prompts []string) {
	if len(layout) == 0 {
		return nil, nil
	}
	files, _, err := readZipEntries(layout)
	if err != nil {
		return nil, nil
	}
	doc := string(files["word/document.xml"])
	seenField := map[string]bool{}
	seenPrompt := map[string]bool{}
	for _, token := range scanLayoutTokens(layoutPlainText(doc), nil) {
		if token.Kind == layoutTokenPrompt {
			if !seenPrompt[token.Inner] {
				seenPrompt[token.Inner] = true
				prompts = append(prompts, token.Inner)
			}
			continue
		}
		if !seenField[token.Inner] {
			seenField[token.Inner] = true
			fields = append(fields, token.Inner)
		}
	}
	return fields, prompts
}

// layoutPlainText — весь текст документа одной строкой: метки в макете
// считаются по нему, а не по отдельным абзацам, чтобы «что система увидела»
// совпадало с тем, что она будет подставлять.
//
// Абзацы склеиваются ВПЛОТНУЮ (`""`, а не `"\n"`): маркер конца строки —
// служебный символ, а не текст документа, и перенос, вставленный между
// абзацами, разорвал бы промт, набранный в двух соседних абзацах. Поля
// макета абзац не пересекают — это и ограничивает склейку безвредной.
func layoutPlainText(docXML string) string {
	var sb strings.Builder
	for _, m := range reWordText.FindAllStringSubmatch(docXML, -1) {
		sb.WriteString(unescapeXML(m[1]))
	}
	return sb.String()
}

func (s *Server) handleGetForm(w http.ResponseWriter, r *http.Request) {
	form, err := s.loadForm(r.Context(), r.URL.Query().Get("mode"))
	if err != nil {
		dbError(w, "get form", err)
		return
	}
	writeJSON(w, http.StatusOK, form)
}

func (s *Server) handleSaveForm(w http.ResponseWriter, r *http.Request) {
	var form OfferForm
	if err := decodeJSON(w, r, &form, 4<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(form.Mode) == "" {
		form.Mode = clientFormMode
	}
	if form.Blocks == nil {
		form.Blocks = []Block{}
	}
	if form.Placeholders == nil {
		form.Placeholders = []Placeholder{}
	}
	if form.RequiredFields == nil {
		form.RequiredFields = []string{}
	}
	if msg := validateForm(form); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}

	placeholders, err := json.Marshal(form.Placeholders)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	blocks, err := json.Marshal(form.Blocks)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	required, err := json.Marshal(form.RequiredFields)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}

	_, err = s.pool.Exec(r.Context(), `
INSERT INTO offers_form (mode, title, placeholders, blocks, required_fields, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (mode) DO UPDATE SET
	title = EXCLUDED.title, placeholders = EXCLUDED.placeholders,
	blocks = EXCLUDED.blocks, required_fields = EXCLUDED.required_fields,
	updated_at = now()`, form.Mode, strings.TrimSpace(form.Title), placeholders, blocks, required)
	if err != nil {
		dbError(w, "save form", err)
		return
	}
	// Макет в JSON формы не приходит и здесь не трогается: его загружают и
	// удаляют отдельными ручками. Сохранение формы не должно его стирать —
	// человек правит блоки чаще, чем макет, и терять загруженный файл при
	// каждом «Сохранить» нельзя.
	var layoutDocx []byte
	var layoutName string
	_ = s.pool.QueryRow(r.Context(), `SELECT layout_docx, layout_name FROM offers_form WHERE mode = $1`,
		form.Mode).Scan(&layoutDocx, &layoutName)
	form.HasLayout = len(layoutDocx) > 0
	form.LayoutName = layoutName
	form.LayoutFields, form.LayoutPrompts = describeLayout(layoutDocx)
	// Отвечаем тем, что реально записали, а НЕ перечитанным значением: пустая
	// форма из БД трактуется как «не настроена» и подменяется формой по
	// умолчанию — админ сохранил бы новую форму и получил ответ со старой.
	saved := form
	saved.ID = 0
	var savedID int64
	var updatedAt string
	if err := s.pool.QueryRow(r.Context(),
		`SELECT id, to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM offers_form WHERE mode = $1`,
		form.Mode).Scan(&savedID, &updatedAt); err != nil {
		dbError(w, "reload form", err)
		return
	}
	saved.ID = savedID
	saved.UpdatedAt = updatedAt
	writeJSON(w, http.StatusOK, saved)
}
