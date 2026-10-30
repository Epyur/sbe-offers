package model

import "encoding/json"

// BriefObjection — возражение клиента и согласованный ответ на него.
// Считается заполненным только когда есть ОБЕ стороны: возражение без ответа —
// это не работа с возражением, а его запись.
type BriefObjection struct {
	Claim    string `json:"claim"`
	Response string `json:"response"`
}

// BriefPayload — структурированный бриф. Порядок полей = порядок тем в
// разговоре (см. skills.BriefTopics); менять его без правки тем не стоит.
type BriefPayload struct {
	ObjectName    string           `json:"object_name"`
	CustomerPains []string         `json:"customer_pains"`
	CustomerGoals []string         `json:"customer_goals"`
	ProjectRisks  []string         `json:"project_risks"`
	Objections    []BriefObjection `json:"objections"`
	BudgetSignal  string           `json:"budget_signal"`
	DecisionMaker string           `json:"decision_maker"`
	Timeline      string           `json:"timeline"`
	WhyUs         []string         `json:"why_us"`
	Notes         string           `json:"notes"`
	// Skipped — темы, от которых менеджер отказался. Без этого признака
	// пропуск неотличим от «поле ещё не заполнено»: значение пусто и до, и
	// после ответа «не знаю», и бот спрашивал бы одно и то же заново.
	Skipped []string `json:"skipped_topics"`
}

// EmptyBriefPayload — бриф с пустыми значениями, но со ВСЕМИ ключами.
// Пустые срезы, а не nil: иначе в JSON уходит null, и клиент падает на
// отрисовке списка.
func EmptyBriefPayload() BriefPayload {
	return BriefPayload{
		CustomerPains: []string{},
		CustomerGoals: []string{},
		ProjectRisks:  []string{},
		Objections:    []BriefObjection{},
		WhyUs:         []string{},
		Skipped:       []string{},
	}
}

// Normalize приводит прочитанный из БД бриф к безопасной форме: nil-срезы
// становятся пустыми.
func (p *BriefPayload) Normalize() {
	if p.CustomerPains == nil {
		p.CustomerPains = []string{}
	}
	if p.CustomerGoals == nil {
		p.CustomerGoals = []string{}
	}
	if p.ProjectRisks == nil {
		p.ProjectRisks = []string{}
	}
	if p.Objections == nil {
		p.Objections = []BriefObjection{}
	}
	if p.WhyUs == nil {
		p.WhyUs = []string{}
	}
	if p.Skipped == nil {
		p.Skipped = []string{}
	}
}

// UnmarshalJSON разбирает бриф и сразу нормализует его. Нормализация живёт
// здесь, а не только в загрузке из БД: иначе любой другой путь разбора
// получает nil-срезы и падает на отрисовке в клиенте.
func (p *BriefPayload) UnmarshalJSON(data []byte) error {
	type plain BriefPayload
	var raw plain
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = BriefPayload(raw)
	p.Normalize()
	return nil
}

// TotalFields — сколько полей брифа всего. Используется в подписи «заполнено
// N из M» в плагине и в решении «бриф готов».
func (p BriefPayload) TotalFields() int {
	return 10
}

// FilledCount — сколько полей реально заполнено. Пустая строка и пустой
// список — незаполненное поле; возражение без ответа — тоже незаполненное.
func (p BriefPayload) FilledCount() int {
	n := 0
	if NonEmpty(p.ObjectName) {
		n++
	}
	if len(p.CustomerPains) > 0 {
		n++
	}
	if len(p.CustomerGoals) > 0 {
		n++
	}
	if len(p.ProjectRisks) > 0 {
		n++
	}
	if hasFullObjection(p.Objections) {
		n++
	}
	if NonEmpty(p.BudgetSignal) {
		n++
	}
	if NonEmpty(p.DecisionMaker) {
		n++
	}
	if NonEmpty(p.Timeline) {
		n++
	}
	if len(p.WhyUs) > 0 {
		n++
	}
	if NonEmpty(p.Notes) {
		n++
	}
	return n
}

// NonEmpty — есть ли в строке хоть один непробельный символ.
func NonEmpty(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

func hasFullObjection(list []BriefObjection) bool {
	for _, o := range list {
		if NonEmpty(o.Claim) && NonEmpty(o.Response) {
			return true
		}
	}
	return false
}
