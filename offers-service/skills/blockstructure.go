package skills

import (
	"fmt"
	"strings"
)

// Промпт «опиши блок словами» (2026-11-05).
//
// Администратор пишет, что должно быть в блоке, а модель отдаёт готовые узлы
// формы. Ограничение одно и главное: только те виды узлов, которые печать
// действительно умеет (docx.go, `renderBlockContentCtx`). Модель, которой не
// сказали список, придумывает `list`, `title`, `text` — и блок молча не
// печатается, а человек не понимает, почему.
//
// Формат ответа — строгий JSON. Модель свободного текста здесь не помощник:
// человек ждёт от редактора готовых полей, а не абзаца прозы.

// blockNodeKinds — виды узлов с их полями, как их понимает печать.
const blockNodeKinds = `- paragraph: {"type":"paragraph","text":"…","align":"center|left|right|justify"}
- heading:   {"type":"heading","text":"…","level":2|3|4}
- bullet_list: {"type":"bullet_list","items":["…","…"]}
- table:     {"type":"table","rows":[["Колонка 1","Колонка 2"],["…","…"]]}
- prompt_text: {"type":"prompt_text","instruction":"что написать модели","maxWords":150}
- image:     {"type":"image","imageOf":"system|material","widthCm":8,"wrap":"right","caption":"…"}
- links:     {"type":"links"}
- page_break: {"type":"page_break"}
- compare_table: {"type":"compare_table","text":"Сравнение систем"}`

// RenderBlockStructureSystem — системный промпт для разбора описания блока.
//
// placeholders — идентификаторы полей формы. Их можно ставить в текст как
// `{{id}}`, и блок сразу будет рабочим; придумывать другие имена нельзя —
// сервер нечего будет подставить, и в документе останется пусто.
func RenderBlockStructureSystem(placeholders []string) string {
	var b strings.Builder
	b.WriteString("ТЫ ПЕРЕВОДИШЬ ОПИСАНИЕ АДМИНИСТРАТОРА В БЛОК КОММЕРЧЕСКОГО ПРЕДЛОЖЕНИЯ.\n")
	b.WriteString("Администратор пишет словами, что должно быть в блоке формы КП. ")
	b.WriteString("Твоя работа — отдать готовые узлы документа, которые редактор покажет полями.\n\n")
	b.WriteString("ВИДЫ УЗЛОВ (только эти, других нет):\n")
	b.WriteString(blockNodeKinds)
	b.WriteString("\n\n")
	if len(placeholders) > 0 {
		b.WriteString("ПОЛЯ ФОРМЫ, которые можно ставить в текст как {{id}}:\n")
		for _, id := range placeholders {
			if id = strings.TrimSpace(id); id != "" {
				b.WriteString("  {{" + id + "}}\n")
			}
		}
		b.WriteString("Других имён полей не придумывай. Если администратор называет поле своими ")
		b.WriteString("словами («название объекта»), ставь это значение в текст обычными словами, ")
		b.WriteString("а не выдуманным полем.\n\n")
	}
	b.WriteString("ПРАВИЛА.\n")
	b.WriteString("1. В блоке ровно то, о чём просили: не добавляй приветствий, выводов и разделов, ")
	b.WriteString("которых нет в описании.\n")
	b.WriteString("2. Текст для клиента пиши по-русски, готовыми фразами. Если администратор ")
	b.WriteString("указал свои слова — сохрани их.\n")
	b.WriteString("3. Где нужен текст модели по данным конкретного КП («опиши систему», ")
	b.WriteString("«обоснуй выбор материалов»), ставь узел prompt_text с подсказкой, ")
	b.WriteString("а не придумывай текст сам: у тебя нет данных предложения.\n")
	b.WriteString("4. Таблицу делай узлом table: первая строка — шапка.\n")
	b.WriteString("5. Порядок узлов — как в описании администратора.\n")
	b.WriteString("6. Заголовок блока — короткий, по делу.\n\n")
	b.WriteString("ФОРМАТ ОТВЕТА: только JSON, без пояснений и без маркеров кода:\n")
	b.WriteString(`{"title":"Заголовок блока","nodes":[ … ],"notes":["чего не понял, если есть"]}`)
	b.WriteString("\n")
	return b.String()
}

// DescribeBlockMaxWords — сколько слов просить у модели для узла по умолчанию.
const DescribeBlockMaxWords = 150

// describeDefaultWordsHint — подсказка, если описание просит длинный текст.
func describeDefaultWordsHint() string {
	return fmt.Sprintf("По умолчанию maxWords=%d.", DescribeBlockMaxWords)
}
