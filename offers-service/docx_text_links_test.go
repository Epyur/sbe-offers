package main

import (
	"strings"
	"testing"
)

// Строка со ссылкой — текст ответа, а не рассуждение (2026-11-06, 0.7.5).
//
// Живой прогон на стенде: макет просит «ссылки сформируй в отдельный список»,
// модель присылает строку «Технический лист: https://nav.tn.ru/…», а чистка
// ответа выбрасывает её — в документе адреса нет вовсе. Причина: чистка строка
// решала по языку (`mostlyCyrillic`), а строка с адресом почти целиком
// латиница, и её принимали за английское рассуждение.
func TestDocumentTextKeepsLinkLines(t *testing.T) {
	answer := "## Ссылки\n\n" +
		"Технический лист: https://nav.tn.ru/pim/ef72eb0891f22018cd27792c390e44bc4e57e39b10000138_R.pdf\n" +
		"Страница системы: https://nav.tn.ru/systems/fundament/tn-fundament-drenazh-prof/"
	got := documentText(stripReasoning(answer))
	if !strings.Contains(got, "https://nav.tn.ru/pim/ef72eb") {
		t.Fatalf("адрес технического листа пропал: %q", got)
	}
	if !strings.Contains(got, "https://nav.tn.ru/systems/fundament/") {
		t.Fatalf("адрес страницы пропал: %q", got)
	}
}

// Адрес посреди английского рассуждения строку не спасает: чистка по-прежнему
// убирает чужие строки. Иначе правило «строка со ссылкой остаётся» отменило бы
// всю чистку — модель пишет адреса и в своих заметках.
func TestDocumentTextStillDropsReasoningWithURL(t *testing.T) {
	answer := "Let me think about the risk group here: https://example.com/x\n" +
		"Площадь горизонтали 4800 м2."
	got := documentText(stripReasoning(answer))
	if strings.Contains(got, "Let me think") {
		t.Fatalf("английское рассуждение с адресом осталось: %q", got)
	}
	if !strings.Contains(got, "4800 м2") {
		t.Fatalf("полезный текст пропал: %q", got)
	}
}

// hasLayoutURL — признак, по которому строка признаётся ссылкой.
func TestHasLayoutURL(t *testing.T) {
	cases := map[string]bool{
		"Технический лист: https://nav.tn.ru/x.pdf": true,
		"https://nav.tn.ru/x": true,
		"Площадь 4800 м2":     false,
		"":                    false,
	}
	for in, want := range cases {
		if got := hasLayoutURL(in); got != want {
			t.Errorf("%q: получили %v, ожидали %v", in, got, want)
		}
	}
}

// Строка с адресом не считается оборванной (2026-11-06, 0.7.5).
//
// trimTruncatedTail режет ответ по последней точке, а в адресе точек сколько
// угодно: «Технический лист: https://nav.tn.ru/pim/x.pdf» превращалось в
// «Технический лист.» — адрес пропадал из документа. Найдено живым прогоном
// на стенде после того, как чистка уже пропускала строку со ссылкой.
func TestTruncatedTailKeepsURL(t *testing.T) {
	answer := "Горизонтальная часть\n" +
		"Площадь 4800 м2.\n" +
		"Технический лист: https://nav.tn.ru/pim/ef72eb_R.pdf"
	got := trimTruncatedTail(answer)
	if !strings.Contains(got, "https://nav.tn.ru/pim/ef72eb_R.pdf") {
		t.Fatalf("адрес отрезан как оборванная фраза: %q", got)
	}
}

// Обрыв посреди слова по-прежнему снимается: правило про адрес не отменяет
// защиту от «…закрывает подземную часть одним кон».
func TestTruncatedTailStillCutsPlainCut(t *testing.T) {
	got := trimTruncatedTail("Комплект закрывает подземную часть одним кон")
	if strings.Contains(got, "одним кон") {
		t.Fatalf("обрыв не снят: %q", got)
	}
}
