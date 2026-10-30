package model

// ContextSectionKeys — известные разделы контекста продукта. Незнакомый ключ
// в PUT — ошибка, а не молчаливая запись: опечатка в имени раздела иначе
// теряет текст.
var ContextSectionKeys = []string{
	"positioning",
	"pains",
	"objections",
	"customer_language",
	"proof",
	"not_promised",
}

// Learning — строка журнала выводов: что узнали, на чём основано, к чему
// относится. Заполняется вручную из плагина; бот читает и учитывает.
type Learning struct {
	Date      string `json:"date"`
	Learning  string `json:"learning"`
	Evidence  string `json:"evidence"`
	AppliesTo string `json:"appliesTo"`
	Status    string `json:"status"` // active | watch | retired
}

// ProductContext — контекст продукта для скилов.
type ProductContext struct {
	ID        int64             `json:"id"`
	Mode      string            `json:"mode"`
	Sections  map[string]string `json:"sections"`
	Learnings []Learning        `json:"learnings"`
	UpdatedBy string            `json:"updatedBy"`
	UpdatedAt string            `json:"updatedAt"`
}

// DefaultContext — что видит администратор, пока ничего не заполнил: все
// разделы есть и пусты. Пустая строка, а не null: плагин рисует textarea.
func DefaultContext(mode string) ProductContext {
	sections := make(map[string]string, len(ContextSectionKeys))
	for _, key := range ContextSectionKeys {
		sections[key] = ""
	}
	return ProductContext{
		Mode:      mode,
		Sections:  sections,
		Learnings: []Learning{},
	}
}

// NormalizeContext добивает отсутствующие разделы и пустой журнал. Записи в
// БД могли появиться до добавления нового раздела — старый контекст должен
// читаться.
func NormalizeContext(c *ProductContext) {
	if c.Sections == nil {
		c.Sections = make(map[string]string, len(ContextSectionKeys))
	}
	for _, key := range ContextSectionKeys {
		if _, ok := c.Sections[key]; !ok {
			c.Sections[key] = ""
		}
	}
	if c.Learnings == nil {
		c.Learnings = []Learning{}
	}
}

// IsContextKey — известен ли раздел.
func IsContextKey(key string) bool {
	for _, k := range ContextSectionKeys {
		if k == key {
			return true
		}
	}
	return false
}

// ValidLearningStatus — допустимый статус вывода.
func ValidLearningStatus(s string) bool {
	return s == "active" || s == "watch" || s == "retired"
}
