package model

import (
	"encoding/json"
	"testing"
)

// Несколько систем в предложении (2026-11-05).
//
// Раньше `Offer.System` был одним значением: сравнить две системы (как
// СТИЛОБАТ V1.0 и V2.0 у конкурента) в одном КП было нельзя. Теперь это
// список. Старые КП хранят время выбора как объект `{"sku":…}` или `{}` —
// такой JSON обязан читаться как список из одного элемента или пустой.

// OfferSystems — выбор систем из JSON системного поля.
func TestOfferSystemsReadsSingleObject(t *testing.T) {
	var got OfferSystems
	if err := json.Unmarshal([]byte(`{"sku":"TN-ROOF-CLASSIC"}`), &got); err != nil {
		t.Fatalf("старый формат не разобран: %v", err)
	}
	if len(got) != 1 || got[0].SKU != "TN-ROOF-CLASSIC" {
		t.Fatalf("старый формат прочитан как %#v", got)
	}
}

// Пустой объект — систем нет, а не «одна пустая».
func TestOfferSystemsReadsEmptyObject(t *testing.T) {
	var got OfferSystems
	if err := json.Unmarshal([]byte(`{}`), &got); err != nil {
		t.Fatalf("пустой объект не разобран: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ожидали пустой список, получили %#v", got)
	}
}

// Массив читается как есть.
func TestOfferSystemsReadsArray(t *testing.T) {
	raw := `[{"noteId":101},{"sku":"TN-ROOF-CLASSIC"}]`
	var got OfferSystems
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("массив не разобран: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ожидали две системы, получили %#v", got)
	}
	if got[0].NoteID != 101 || got[1].SKU != "TN-ROOF-CLASSIC" {
		t.Fatalf("порядок или значения сбились: %#v", got)
	}
}

// null и пустая строка — пустой список, а не паника.
func TestOfferSystemsReadsNullOrEmpty(t *testing.T) {
	for _, raw := range []string{`null`, `""`} {
		var got OfferSystems
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if len(got) != 0 {
			t.Fatalf("%s: ожидали пустой список, получили %#v", raw, got)
		}
	}
}

// Список пишется массивом: следующий клиент не должен получить объект.
func TestOfferSystemsMarshalsArray(t *testing.T) {
	systems := OfferSystems{{SKU: "A"}, {NoteID: 7}}
	raw, err := json.Marshal(systems)
	if err != nil {
		t.Fatalf("запись: %v", err)
	}
	if string(raw) != `[{"sku":"A"},{"noteId":7}]` {
		t.Fatalf("записано как %s", raw)
	}
}

// Пустой список пишется как `[]`, а не `null`: иначе клиент снова увидит
// непривычный тип и упадёт на разборе.
func TestEmptyOfferSystemsMarshalsArray(t *testing.T) {
	raw, err := json.Marshal(OfferSystems{})
	if err != nil {
		t.Fatalf("запись: %v", err)
	}
	if string(raw) != `[]` {
		t.Fatalf("записано как %s", raw)
	}
}

// Same — одна и та же система: код главнее заметки, сравнение без регистра.
func TestOfferSystemSame(t *testing.T) {
	a := OfferSystem{SKU: " tn-roof-classic "}
	b := OfferSystem{SKU: "TN-ROOF-CLASSIC"}
	if !a.Same(b) {
		t.Fatal("код с пробелом и регистром должен совпадать")
	}
	if (OfferSystem{NoteID: 5}).Same(OfferSystem{NoteID: 6}) {
		t.Fatal("разные заметки совпали")
	}
	if (OfferSystem{NoteID: 5}).Same(OfferSystem{NoteID: 5}) == false {
		t.Fatal("одна заметка не совпала сама с собой")
	}
}

// Add не дублирует систему и сохраняет порядок добавления.
func TestOfferSystemsAdd(t *testing.T) {
	var list OfferSystems
	list = list.Add(OfferSystem{NoteID: 1})
	list = list.Add(OfferSystem{NoteID: 2})
	list = list.Add(OfferSystem{NoteID: 1})
	if len(list) != 2 {
		t.Fatalf("дубль не отсечён: %#v", list)
	}
	if list[0].NoteID != 1 || list[1].NoteID != 2 {
		t.Fatalf("порядок сбился: %#v", list)
	}
}

// Remove убирает систему и оставляет остальные.
func TestOfferSystemsRemove(t *testing.T) {
	list := OfferSystems{{NoteID: 1}, {NoteID: 2}, {NoteID: 3}}
	got := list.Remove(OfferSystem{NoteID: 2})
	if len(got) != 2 || got[0].NoteID != 1 || got[1].NoteID != 3 {
		t.Fatalf("удаление сломало список: %#v", got)
	}
}
