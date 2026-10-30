package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Разбор JSON из ответа модели.
//
// Перенесено из `sbe-llm/src/services/llm-center.ts` (extractJsonBlock +
// repairJson) и `sbe-photobank/src/services/ai-describe.service.ts`. Причина
// переноса — история этого кода: «не разобрал» составляло около 90% жалоб на
// ИИ-описания, поэтому там разбор сознательно ТЕРПИМЫЙ:
//
//  1. ответ обрезан или обёрнут — сначала вырезаем fenced-блок ```json … ```;
//  2. режем от первой скобки ({ или [, что раньше) до ПАРНОЙ закрывающей;
//     резать «от { до последней }» нельзя — на массиве объектов
//     ([{…},{…}]) это даёт «Unexpected non-whitespace character after JSON»;
//  3. обрезанный по лимиту токенов ответ достраиваем (закрываем строку и
//     скобки) и отдаём частичный результат: лучше половина брифа, чем
//     «не разобрал» на весь ответ;
//  4. отказ — только когда JSON в ответе нет вовсе.
//
// Вторая грабля: в Фотобанке лежит УПРОЩЁННАЯ копия этого разбора без
// ремонта, и две реализации одной задачи разошлись. Здесь копии нет —
// единственная точка.

// errNoJSON — в ответе модели нет ни объекта, ни массива.
var errNoJSON = errors.New("в ответе модели нет JSON")

// jsonPreview обрезает ответ для журнала и сообщения об ошибке: в нём бывает
// эхо промпта с данными заказчика.
func jsonPreview(text string, max int) string {
	flat := strings.Join(strings.Fields(text), " ")
	if flat == "" {
		return "(пустой ответ)"
	}
	if len([]rune(flat)) <= max {
		return flat
	}
	return string([]rune(flat)[:max]) + fmt.Sprintf(": (ещё %d символов)", len([]rune(flat))-max)
}

// extractJSON вытаскивает первый объект или массив из ответа модели.
// Возвращает разобранное значение; при обрезке — частичное и флаг repaired.
func extractJSON(text string) (value any, repaired bool, err error) {
	cleaned := strings.TrimSpace(text)
	if fence := fenceBlock(cleaned); fence != "" {
		cleaned = strings.TrimSpace(fence)
	}

	objStart := strings.Index(cleaned, "{")
	arrStart := strings.Index(cleaned, "[")
	// Массив — только если он начинается раньше объекта. Иначе на ответе
	// [{…},{…}] вырежется внутренний объект, и разбор упадёт на хвосте.
	isArray := arrStart != -1 && (objStart == -1 || arrStart < objStart)
	var open, closeCh byte = '{', '}'
	start := objStart
	if isArray {
		open, closeCh = '[', ']'
		start = arrStart
	}
	if start == -1 {
		return nil, false, fmt.Errorf("%w: %s", errNoJSON, jsonPreview(cleaned, 300))
	}

	if end := pairedEnd(cleaned, start, open, closeCh); end > start {
		candidate := cleaned[start : end+1]
		var v any
		if err := json.Unmarshal([]byte(candidate), &v); err == nil {
			return v, false, nil
		}
		// Скобки парные, но внутри битый JSON: чаще всего хвостовая запятая
		// перед закрывающей скобкой («[…]», «{…,}») — модель так заканчивает
		// списки. Чиним запятые и пробуем снова, прежде чем звать полный ремонт.
		if stripped := stripTrailingCommas(candidate); stripped != candidate {
			if err := json.Unmarshal([]byte(stripped), &v); err == nil {
				return v, true, nil
			}
		}
		if v, ok := repairJSON(candidate); ok {
			return v, true, nil
		}
		return nil, false, fmt.Errorf("ответ модели не разобрался как JSON: %s", jsonPreview(cleaned, 300))
	}

	// Закрывающей скобки нет — ответ обрезан по лимиту токенов. Достраиваем.
	if v, ok := repairJSON(cleaned[start:]); ok {
		return v, true, nil
	}
	return nil, false, fmt.Errorf("ответ модели обрезан и не восстановлен: %s", jsonPreview(cleaned, 300))
}

// fenceBlock возвращает содержимое первого ```json … ``` или ``` … ``` блока.
// Пустая строка — блока нет.
func fenceBlock(s string) string {
	idx := strings.Index(s, "```")
	if idx < 0 {
		return ""
	}
	rest := s[idx+3:]
	// Пропускаем метку языка до первой перевода строки.
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		firstLine := strings.TrimSpace(rest[:nl])
		if strings.EqualFold(firstLine, "json") || firstLine == "" {
			rest = rest[nl+1:]
		}
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		// Незакрытый fenced-блок — это тоже обрезка: отдаём остаток, ремонт
		// разберётся дальше.
		return rest
	}
	return rest[:end]
}

// stripTrailingCommas убирает запятые перед закрывающей скобкой, не трогая
// строки. «[…,», «{…,» — самая частая мелочь от модели.
func stripTrailingCommas(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			b.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
			b.WriteByte(c)
		case '}', ']':
			// Снимаем запятую и пробелы, уже записанные перед скобкой.
			out := strings.TrimRight(b.String(), " \t\r\n,")
			b.Reset()
			b.WriteString(out)
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// pairedEnd ищет закрывающую скобку того же типа с учётом вложенности и строк.
// Возвращает -1, если пара не найдена.
func pairedEnd(s string, start int, open, closeCh byte) int {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case open:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// repairJSON достраивает обрезанный JSON: закрывает незакрытую строку,
// убирает висящую запятую и закрывает все открытые скобки в обратном порядке.
// Возвращает false, если из текста не выходит ничего разбираемого.
//
// Перенесено из llm-center.ts:repairJson.
func repairJSON(text string) (any, bool) {
	var stack []byte
	inString := false
	escaped := false
	// safeEnd — граница безопасного обрезания: последняя точка, где структура
	// заведомо целая (после запятой или закрытой скобки).
	safeEnd := -1
	for i := 0; i < len(text); i++ {
		c := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
				safeEnd = i + 1
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 {
				continue
			}
			stack = stack[:len(stack)-1]
			safeEnd = i + 1
		case ',':
			safeEnd = i + 1
		}
	}

	// Собираем кандидатов: сначала как есть с достройкой, потом обрезанные до
	// безопасной границы — от длинного к короткому.
	candidates := []string{buildRepaired(text, stack, inString, escaped)}
	if safeEnd > 0 && safeEnd < len(text) {
		trimmed := strings.TrimRight(text[:safeEnd], " \t\r\n,")
		if trimmed != text {
			candidates = append(candidates, buildRepaired(trimmed, openStack(trimmed), false, false))
		}
	}
	for _, cand := range candidates {
		var v any
		if err := json.Unmarshal([]byte(cand), &v); err == nil {
			return v, true
		}
	}
	return nil, false
}

// openStack пересчитывает стек закрывающих скобок для уже обрезанного текста.
func openStack(text string) []byte {
	var stack []byte
	inString := false
	escaped := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return stack
}

// buildRepaired закрывает строку и скобки.
func buildRepaired(text string, stack []byte, inString, escaped bool) string {
	out := strings.TrimRight(text, " \t\r\n")
	if inString {
		if escaped {
			// Висящий обратный слэш сломал бы строку — убираем.
			out = strings.TrimSuffix(out, "\\")
		}
		out += `"`
	}
	out = strings.TrimRight(out, " \t\r\n,")
	for i := len(stack) - 1; i >= 0; i-- {
		out += string(stack[i])
	}
	return out
}
