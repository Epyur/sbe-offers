package skills

import (
	"testing"

	"mailers/offers/model"
)

// Порядок разговора (2026-11-01): контекст объекта и адресата идёт ПЕРВЫМ.
// Бот не спрашивает позиции, пока не знает, для чего и кому предложение, —
// поэтому темы начинаются с объекта, а не с задач.
func TestBriefTopicsStartWithObjectContext(t *testing.T) {
	topics := BriefTopics()
	if len(topics) == 0 {
		t.Fatal("манифест тем пуст")
	}
	if topics[0].Key != "object_name" || topics[0].Stage != StageObject {
		t.Fatalf("первая тема — %q этапа %q", topics[0].Key, topics[0].Stage)
	}
	// Этапы идут по порядку: объект, сделка, что предлагаем, условия.
	want := []string{StageObject, StageDeal, StageOffer, StageTerms}
	seen := []string{}
	for _, tp := range topics {
		if len(seen) == 0 || seen[len(seen)-1] != tp.Stage {
			seen = append(seen, tp.Stage)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("этапы в разговоре: %v, ожидали %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("этапы в разговоре: %v, ожидали %v", seen, want)
		}
	}
}

// Этап закрывается только вместе со всеми своими темами: заполненный объект
// без назначения и площади оставляет разговор на этапе «Объект и адресат».
func TestCurrentStageHoldsUntilStageClosed(t *testing.T) {
	p := model.EmptyBriefPayload()
	p.ObjectName = "ЖК Поэма"
	if got := CurrentStage(p); got != StageObject {
		t.Fatalf("ожидали этап %q, получили %q", StageObject, got)
	}
	p.ObjectPurpose = "жилой дом"
	p.ObjectArea = "1200 м²"
	p.CustomerAddress = "ООО Строймонтаж"
	if got := CurrentStage(p); got != StageDeal {
		t.Fatalf("объект собран — ожидали этап %q, получили %q", StageDeal, got)
	}
}

// Системы спрашиваются РАНЬШЕ условий: «что предлагаем» важнее бюджета, а
// позиции собираются вообще после всех тем.
func TestPlannedSystemsComeBeforeTerms(t *testing.T) {
	if got := StageOf("planned_systems"); got != StageOffer {
		t.Fatalf("системы: этап %q, ожидали %q", got, StageOffer)
	}
	if got := StageOf("budget_signal"); got != StageTerms {
		t.Fatalf("бюджет: этап %q, ожидали %q", got, StageTerms)
	}
}

// Пройденный бриф — все темы закрыты, текущего этапа нет: дальше позиции.
func TestCurrentStageEmptyWhenBriefDone(t *testing.T) {
	p := model.EmptyBriefPayload()
	for _, tp := range BriefTopics() {
		p.Skipped = append(p.Skipped, tp.Key)
	}
	if got := CurrentStage(p); got != "" {
		t.Fatalf("бриф пройден — этапа быть не должно, получили %q", got)
	}
}

// Этапы с подписями — для сообщения бота «Этап 1 из 4: Объект и адресат».
func TestStagesHaveLabels(t *testing.T) {
	stages := Stages()
	if len(stages) != 4 {
		t.Fatalf("ожидали 4 этапа, получили %d", len(stages))
	}
	for _, s := range stages {
		if s.Label == "" {
			t.Errorf("у этапа %q нет подписи", s.Key)
		}
	}
	if StageLabel(StageOffer) == "" {
		t.Error("у этапа «что предлагаем» нет подписи")
	}
}
