package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mailers/offers/model"
)

// Тексты системы (2026-11-06, шаг B): хеш карточки, промпт блока и чистка.
//
// Таблица `offers_system_text` живёт в БД, её поведение проверяется на стенде
// (offers_local_e2e.mjs, раздел 23). Здесь — то, что можно проверить без
// Postgres: правило «какой текст свежий» и то, что генерация не падает на
// аварии модели.

// systemTextTestCard — карточка, по которой считается хеш.
func systemTextTestCard() SystemCard {
	return SystemCard{
		NoteID:      5874,
		Title:       "ТН-ФУНДАМЕНТ Дренаж Проф",
		Description: "Система гидроизоляции фундамента.",
		Composition: []string{"Подготовка основания", "Мембрана Logicbase V-SL"},
		Specifications: [][]string{
			{"Показатель", "Значение"},
			{"Прочность на растяжение", "12 МПа"},
		},
	}
}

// Хеш меняется вместе с содержимым карточки: иначе «пересчитать» решало бы,
// что текст свежий, и правка в Базе знаний не доходила бы до документа.
func TestSystemTextHashTracksCardContent(t *testing.T) {
	card := systemTextTestCard()
	base := systemCardHash(card)
	if base == "" {
		t.Fatal("хеш пуст: свежесть текста нечем проверять")
	}
	if again := systemCardHash(card); again != base {
		t.Fatalf("хеш не воспроизводится: %q != %q", again, base)
	}
	changed := card
	changed.Description = card.Description + " Вторая редакция."
	if systemCardHash(changed) == base {
		t.Fatal("правка описания не изменила хеш: устаревший текст остался бы «свежим»")
	}
}

// Хеш блока включает инструкцию: переписали просьбу — текст к ней не
// относится, и это должно быть видно по хешу, а не по бессмысленному совпадению.
func TestSystemTextHashSeparatesBlocks(t *testing.T) {
	card := systemTextTestCard()
	desc := systemTextHash(card, systemTextDescription)
	adv := systemTextHash(card, systemTextAdvantages)
	if desc == adv {
		t.Fatal("хеши двух блоков совпали: блоку достался бы текст соседа")
	}
}

// Данные предложения в хеш карточки не входят: один и тот же текст системы
// годится для всех КП, иначе модель вызывалась бы заново на каждое предложение.
func TestSystemCardHashIgnoresOfferData(t *testing.T) {
	card := systemTextTestCard()
	if systemCardHash(card) != systemCardHash(systemTextTestCard()) {
		t.Fatal("хеш карточки зависит от чего-то ещё — текст переписывался бы без причины")
	}
}

// writeSystemText идёт ТЕМ ЖЕ путём, что блок формы: инструкция → system,
// данные предложения → user, ответ чистится. Проверяем, что карточка дошла до
// промпта и что рассуждение модели в текст не попало.
func TestWriteSystemTextSendsCardAndCleansAnswer(t *testing.T) {
	card := systemTextTestCard()
	c := &fakeCompleter{answer: "The task is to describe.\n" +
		"## Обоснование применения\n" +
		"Система защищает подземную часть дома от грунтовых вод и закрывает весь контур."}
	got, err := writeSystemTextWith(context.Background(), systemTextInstruction[systemTextDescription],
		Offer{Number: "003/2026", Client: model.OfferClient{Name: "Примерная"}}, card, nil, c)
	if err != nil {
		t.Fatalf("writeSystemText: %v", err)
	}
	if strings.Contains(got, "The task") {
		t.Fatalf("рассуждение модели попало в текст: %q", got)
	}
	if !strings.Contains(got, "Система защищает") {
		t.Fatalf("полезный текст потерян: %q", got)
	}
	if strings.Contains(got, "##") {
		t.Fatalf("разметка заголовка осталась в тексте: %q", got)
	}
}

// Авария модели — пустой текст, а не паника: выбор системы не должен
// проваливаться из-за недоступной модели.
func TestWriteSystemTextReturnsErrorOnModelFailure(t *testing.T) {
	_, err := writeSystemTextWith(context.Background(), systemTextInstruction[systemTextAdvantages],
		Offer{}, systemTextTestCard(), nil, failingCompleter{})
	if !errors.Is(err, errLLMNotConfigured) {
		t.Fatalf("ожидали ошибку модели, получили %v", err)
	}
}

// failingCompleter — модель, которая отвечает «ключ не настроен»: так ведёт
// себя llmCompleter без LLM_SERVICE_SECRET.
type failingCompleter struct{}

func (failingCompleter) Complete(context.Context, string, string, int) (string, error) {
	return "", errLLMNotConfigured
}

// Пределы ответа по видам работы: сборка документа ждёт модель, обычные ручки
// нет. Стоит проверить, потому что это единственный оставшийся предел:
// `WriteTimeout` сервера снят, и ошибка здесь означала бы тишину в боте снова.
func TestWriteTimeoutForRoutes(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/api/offers/bot/offers/8/docx", "10m0s"},
		{"/api/offers/offers/8/export?format=docx", "10m0s"},
		{"/api/offers/offers/8/system", "7m0s"},
		{"/api/offers/offers/8/system/5874/recalculate", "7m0s"},
		{"/api/offers/offers", "1m0s"},
		{"/api/offers/health", "1m0s"},
	}
	for _, tc := range cases {
		if got := writeTimeoutFor(tc.path).String(); got != tc.want {
			t.Errorf("%s: ожидали %s, получили %s", tc.path, tc.want, got)
		}
	}
}
