package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// «Опиши блок словами» — модель предлагает структуру блока (2026-11-05).
//
// Жалоба пользователя: «сейчас настройка блоков крайне не дружелюбная».
// Настраивать блок в редакторе — это выбрать тип узла, набрать текст, добавить
// таблицу, расставить отступы. Администратор думает не так: он знает, ЧТО
// должно быть в блоке («таблица с составом, потом два абзаца о преимуществах»),
// а не как это разложить по узлам.
//
// Здесь одно: свободное описание → готовый набор узлов. Дальше человек
// дооформляет их руками в том же редакторе — правка остаётся за ним, и ничего
// не перезаписывается без показа.

// describeBlockRequest — что прислал плагин.
type describeBlockRequest struct {
	// Description — слова администратора.
	Description string `json:"description"`
	// Placeholders — идентификаторы полей формы: модель может расставить их в
	// тексте, и тогда первое сохранение сразу даёт рабочий блок.
	Placeholders []string `json:"placeholders"`
}

// describeBlockResponse — готовый блок.
type describeBlockResponse struct {
	Title string      `json:"title"`
	Nodes []blockNode `json:"nodes"`
	// Notes — чего не поняли (или что модель пропустила). Не ошибка: блок
	// показывается, а человека предупреждают, что проверить.
	Notes []string `json:"notes,omitempty"`
}

// errEmptyDescription — модель ответила, но ни одного годного узла не дала.
var errEmptyDescription = errors.New("модель не предложила ни одного узла — опишите блок иначе")

// handleDescribeBlock — POST /api/offers/form/describe.
func (s *Server) handleDescribeBlock(w http.ResponseWriter, r *http.Request) {
	var req describeBlockRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "опишите, что должно быть в блоке"})
		return
	}
	resp, err := describeBlock(r.Context(), description, req.Placeholders, llmCompleter{})
	if err != nil {
		// Модель недоступна — это не 500: человеку надо увидеть причину и
		// возможность настроить блок руками, как раньше.
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "модель недоступна: " + errorText(err),
		})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// describeBlock собирает блок из свободного описания.
//
// Ответ модели — JSON. Его разбор терпимый (jsonrepair, как у брифа): модель
// любит обрамлять ответ пояснениями и хранит числа строками. Не разобрали —
// возвращаем ошибку, а не пустой блок: пустой блок человек примет за «система
// ничего не поняла», хотя дело в формате ответа.
func describeBlock(ctx context.Context, description string, placeholders []string, c Completer) (describeBlockResponse, error) {
	system := skills.RenderBlockStructureSystem(placeholders)
	raw, err := c.Complete(ctx, system, description, 2000)
	if err != nil {
		return describeBlockResponse{}, err
	}
	value, _, err := model.ExtractJSON(raw)
	if err != nil {
		return describeBlockResponse{}, err
	}
	parsed := parseDescribedBlock(value)
	nodes := sanitizeDescribedNodes(parsed.Nodes)
	if len(nodes) == 0 {
		return describeBlockResponse{}, errEmptyDescription
	}
	title := strings.TrimSpace(parsed.Title)
	if title == "" {
		title = "Новый блок"
	}
	return describeBlockResponse{Title: title, Nodes: nodes, Notes: parsed.Notes}, nil
}

// describedBlock — то, что вернула модель, до приведения узлов.
type describedBlock struct {
	Title string
	Nodes []blockNode
	Notes []string
}

// parseDescribedBlock понимает оба вида ответа.
//
// Модели отдают узлы и обёрнутыми в `{title, nodes, notes}`, и голым списком —
// «две формы одного ответа» ловились на брифе, и здесь проще оказалось
// понимать обе, чем объяснять модели, какая правильная.
func parseDescribedBlock(value any) describedBlock {
	switch v := value.(type) {
	case []any:
		return describedBlock{Nodes: nodesFromJSON(v)}
	case map[string]any:
		out := describedBlock{}
		if nodes, ok := v["nodes"].([]any); ok {
			out.Nodes = nodesFromJSON(nodes)
		}
		out.Title, _ = v["title"].(string)
		out.Notes = stringList(v["notes"])
		return out
	default:
		return describedBlock{}
	}
}

// stringList — строки из значения JSON; всё остальное отбрасывается.
func stringList(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// nodesFromJSON превращает разобранный JSON в узлы.
//
// Через повторную сериализацию, а не ручной разбор каждой ветки: узлов девять
// видов, и держать их разбор вручную значило бы дублировать структуру блока
// (а значит — расходиться с ней при каждом новом виде).
func nodesFromJSON(raw []any) []blockNode {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var nodes []blockNode
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil
	}
	return nodes
}

// sanitizeDescribedNodes приводит узлы модели к тому, что умеет печать.
//
// Модель может придумать узел, которого нет (`list`, `text`, `title`), и
// тогда печать молча пропустит блок. Поэтому неизвестный тип приводится к
// абзацу, а пустые узлы выбрасываются: администратор увидит в редакторе ровно
// то, что потом попадёт в документ.
func sanitizeDescribedNodes(nodes []blockNode) []blockNode {
	out := make([]blockNode, 0, len(nodes))
	for _, node := range nodes {
		switch node.Type {
		case "paragraph", "heading", "bullet_list", "table", "page_break",
			"links", "prompt_text", "image", "compare_table":
			// известный вид — как есть
		case "list", "bullet", "список":
			node.Type = "bullet_list"
		case "text", "title", "заголовок", "":
			if strings.EqualFold(node.Type, "heading") || strings.EqualFold(node.Type, "заголовок") {
				node.Type = "heading"
			} else {
				node.Type = "paragraph"
			}
		default:
			node.Type = "paragraph"
		}
		if !nodeHasContent(node) {
			continue
		}
		out = append(out, node)
	}
	return out
}

// nodeHasContent — узел что-то содержит.
func nodeHasContent(node blockNode) bool {
	switch node.Type {
	case "page_break", "links":
		return true
	case "prompt_text":
		return strings.TrimSpace(node.Instruction) != ""
	case "bullet_list":
		return len(node.Items) > 0
	case "table":
		return len(node.Rows) > 0
	case "image":
		return strings.TrimSpace(node.ImageKey) != "" || imageOf(node.ImageOf) != imageOfStatic
	case "compare_table":
		return true
	default:
		return strings.TrimSpace(node.Text) != ""
	}
}
