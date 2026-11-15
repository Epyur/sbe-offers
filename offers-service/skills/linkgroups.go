package skills

import "strings"

// Правила группировки ссылок — ДАННЫЕ формы (2026-11-14, дефект 0.10.2).
//
// До этой правки раскладка ссылок по группам была зашита в коде
// (`cardRefGroupOf`, `CardRefGroupTitle`, `cardRefGroupRank`), и чтобы поменять
// способ группировки, администратору требовался разработчик. Пользователь:
// «сделать блок ссылок редактируемым… сейчас чтобы поменять способ группировки
// нужно менять код». Решение зафиксировано: правило задаёт администратор, ИИ
// его не сочиняет.
//
// Что переехало в данные: порядок групп, их id и подписи, и по каким признакам
// ссылка попадает в группу. Что осталось кодом: сборка ссылок с карточек,
// дедупликация, сортировка внутри группы по подписи.
//
// Признаки, по которым правило узнаёт ссылку: её подпись и адрес. Тип карточки
// сюда не входит намеренно — ссылка после `ParseCardRefs` уже не знает, откуда
// пришла, и второй источник «вида» разошёлся бы с первым.
//
// Умолчание (`DefaultLinkGroupRules`) равно предыдущему поведению до запятой:
// старой форме, где правил нет, печатается то же, что печаталось вчера. Это
// требование плана: существующие документы не меняются молча.

// LinkGroupRule — одна группа ссылок: как её узнать и как назвать.
//
// Порядок ссылок в разделе — порядок групп в форме: администратор двигает
// группу вверх, и она печатается раньше. Сам срез поэтому и есть порядок.
type LinkGroupRule struct {
	// ID — внутренний ключ группы (латиницей): по нему группируют и его
	// проверяют. Виден только администратору в редакторе.
	ID string `json:"id"`
	// Title — заголовок группы в документе.
	Title string `json:"title"`
	// URLContains — адрес содержит любой из кусков (регистр не важен). Пусто —
	// признак по адресу не проверяется.
	URLContains []string `json:"urlContains,omitempty"`
	// LabelContains — подпись содержит любой из кусков (регистр не важен).
	LabelContains []string `json:"labelContains,omitempty"`
}

// LinkGroupRules — правила группировки ссылок формы.
//
// Срез, а не карта: порядок групп — это и есть порядок печати, а карта его не
// хранит. Последняя группа играет роль «остальное»: ссылка, не подошедшая ни
// одному правилу выше, попадает в неё. Именно так было и в коде — «сертификаты
// и прочее» стояли по умолчанию.
type LinkGroupRules []LinkGroupRule

// DefaultLinkGroupRules — правила по умолчанию, равные прежнему поведению.
//
// Значения кусков совпадают с `cardRefGroupOf` до правки. Правила: страница
// навигатора по адресу; СТО, узлы и техлист по подписи; всё прочее —
// сертификаты. Умолчание стоит в `defaultForm`, поэтому старая форма получает
// его и не меняет ни одного печатного документа.
func DefaultLinkGroupRules() LinkGroupRules {
	return LinkGroupRules{
		{
			ID:          RefGroupPage,
			Title:       "Страницы продуктов",
			URLContains: []string{"/systems/", "/catalog/", "/catalogue/"},
		},
		{ID: RefGroupSTO, Title: "Стандарты организации (СТО)", LabelContains: []string{"сто "}},
		{ID: RefGroupTechList, Title: "Технические листы", LabelContains: []string{
			"техлист", "тех. лист", "тех.лист", "технический лист",
		}},
		{ID: RefGroupUzly, Title: "Узлы", LabelContains: []string{"узл"}},
		{ID: RefGroupCert, Title: "Сертификаты и декларации"},
	}
}

// ValidateLinkGroupRules — что не так с правилами; пусто — правила годны.
//
// Пустой набор — законное состояние: тогда действуют правила по умолчанию, и
// старая форма остаётся действительной. Остальные ошибки называются человеку,
// который правил форму: пустой id или заголовок (группа без подписи), повтор
// id (две группы не отличить), нет группы «остальное» (ссылка пропала бы
// молча — худший вид потери).
func ValidateLinkGroupRules(rules LinkGroupRules) string {
	if len(rules) == 0 {
		return ""
	}
	seen := make(map[string]bool, len(rules))
	for _, rule := range rules {
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			return "у группы ссылок пустой id"
		}
		if strings.TrimSpace(rule.Title) == "" {
			return "у группы ссылок «" + id + "» пустой заголовок"
		}
		if seen[id] {
			return "группа ссылок «" + id + "» повторяется"
		}
		seen[id] = true
	}
	last := rules[len(rules)-1]
	if len(nonEmpty(last.URLContains)) > 0 || len(nonEmpty(last.LabelContains)) > 0 {
		return "последняя группа ссылок должна быть общей «остальное»: " +
			"без признаков, иначе ссылка, не подошедшая ни одному правилу, пропадёт из документа"
	}
	return ""
}

// LinkGroupOf — группа ссылки по правилам формы: первое правило, подошедшее
// по адресу или подписи. Правила смотрятся в порядке формы; последнее обычно
// без признаков и забирает всё остальное.
//
// Помеченная группа на ссылке здесь НЕ учитывается: это старое правило,
// вшитое в код, и принимать его за настройку администратора значило бы никогда
// не дать правилам сработать на живом кеше карточек. Пометку уважает
// `groupOfRef` — но только когда правил нет.
func LinkGroupOf(ref CardRef, rules LinkGroupRules) string {
	if len(rules) == 0 {
		rules = DefaultLinkGroupRules()
	}
	url := lowerTrim(ref.URL)
	label := lowerTrim(ref.Label)
	for _, rule := range rules {
		if matchesAny(url, rule.URLContains) || matchesAny(label, rule.LabelContains) {
			return rule.ID
		}
	}
	// Правила без общей группы: ссылка всё равно должна где-то напечататься —
	// терять её нельзя. Отдаём последней группе.
	return defaultLinkGroupID(rules)
}

// LinkGroupTitle — заголовок группы по правилам формы; неизвестная группа
// (ссылка со старым кешем, правило убрали) подписывается своим id, а не
// пустотой: пустой заголовок в документе выглядит как сбой вёрстки.
func LinkGroupTitle(group string, rules LinkGroupRules) string {
	if len(rules) == 0 {
		rules = DefaultLinkGroupRules()
	}
	for _, rule := range rules {
		if rule.ID == group {
			return rule.Title
		}
	}
	return group
}

// linkGroupRankOf — место группы в порядке печати: меньше — раньше. Номер —
// позиция правила в форме; неизвестная группа уходит в конец.
func linkGroupRankOf(group string, rules LinkGroupRules) int {
	for i, rule := range rules {
		if rule.ID == group {
			return i
		}
	}
	return len(rules)
}

// GroupCardRefsWith раскладывает ссылки на группы ФОРМЫ в порядке формы.
//
// Вход берётся уже отсортированным (`SortCardRefsWith` с теми же правилами):
// здесь только разрезание на группы для печати заголовков. Пустые группы не
// попадают — заголовок без ссылок читается как пропуск.
func GroupCardRefsWith(refs []CardRef, rules LinkGroupRules) []CardRefGroup {
	if len(refs) == 0 {
		return nil
	}
	if len(rules) == 0 {
		rules = DefaultLinkGroupRules()
	}
	var out []CardRefGroup
	index := make(map[string]int, len(rules))
	for _, ref := range refs {
		group := groupOfRef(ref, rules)
		i, ok := index[group]
		if !ok {
			index[group] = len(out)
			out = append(out, CardRefGroup{Group: group, Title: LinkGroupTitle(group, rules)})
			i = len(out) - 1
		}
		out[i].Refs = append(out[i].Refs, ref)
	}
	return out
}

// groupOfRef — группа ссылки для сортировки и печати.
//
// Одно правило на два вопроса. Явные правила формы главнее пометки: пометка —
// это старое правило, вшитое в код, и ссылки из кеша карточек приходят уже
// помеченными. Правила по умолчанию пометку уважают: иначе разбор по адресу не
// отличил бы узлы от техлистов (оба лежат на /pim/…pdf) и развёл бы ссылки из
// одной группы по разным местам документа.
func groupOfRef(ref CardRef, rules LinkGroupRules) string {
	if rulesExplicit(rules) {
		return LinkGroupOf(ref, rules)
	}
	if ref.Group != "" {
		return ref.Group
	}
	return LinkGroupOf(ref, rules)
}

// rulesExplicit — правила заданы администратором, а не взяты из умолчания.
//
// Форма по умолчанию приходит с DefaultLinkGroupRules(), и отличить её от
// настроенных правил можно только сравнением: по порядку, id, заголовкам и
// кускам. Любое другое правило считается настройкой.
func rulesExplicit(rules LinkGroupRules) bool {
	if len(rules) == 0 {
		return false
	}
	def := DefaultLinkGroupRules()
	if len(rules) != len(def) {
		return true
	}
	for i := range def {
		if !sameRule(rules[i], def[i]) {
			return true
		}
	}
	return false
}

// sameRule — совпадают ли правила полностью: id, заголовок и оба списка кусков.
func sameRule(a, b LinkGroupRule) bool {
	if a.ID != b.ID || a.Title != b.Title {
		return false
	}
	if len(a.URLContains) != len(b.URLContains) || len(a.LabelContains) != len(b.LabelContains) {
		return false
	}
	for i := range a.URLContains {
		if a.URLContains[i] != b.URLContains[i] {
			return false
		}
	}
	for i := range a.LabelContains {
		if a.LabelContains[i] != b.LabelContains[i] {
			return false
		}
	}
	return true
}

// matchesAny — содержит ли строка (уже в нижнем регистре) любой из кусков.
// Пустые куски и куски из одних пробелов не учитываются: строка «» совпадала
// бы со всем и забрала бы все ссылки в первую группу.
func matchesAny(lower string, needles []string) bool {
	for _, needle := range needles {
		if n := lowerTrim(needle); n != "" && strings.Contains(lower, n) {
			return true
		}
	}
	return false
}

// lowerTrim — ключ сравнения подписи и адреса: регистр и внешние пробелы не
// важны. «ё» остаётся как есть: в адресах её нет, а в подписи замена «ё»→«е»
// уже сделана раньше (`normalizeCardRefLabel`), и вторая замена здесь спутала
// бы правила.
func lowerTrim(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// nonEmpty — куски без пустых и пробельных значений. Пустой кусок совпал бы со
// всем и забрал бы все ссылки в одну группу.
func nonEmpty(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return out
}

// defaultLinkGroupID - ключ группы остальное: последнее правило.
func defaultLinkGroupID(rules LinkGroupRules) string {
	return rules[len(rules)-1].ID
}
