// Package model — данные предметной области, общие для сервиса, скилов и
// чистых модулей. Отдельный пакет, а не файлы в main: main нельзя
// импортировать, и промпты/чек-листы без этого не проверяются тестами.
package model

import (
	"encoding/json"
	"strings"
)

// OfferItem — позиция предложения. Это СНИМОК: цена и сумма фиксируются в
// момент добавления карточки, поэтому последующая правка каталога на старые
// предложения не влияет.
type OfferItem struct {
	CatalogID *int64 `json:"catalogId,omitempty"`
	// EKN — номер ЕКН материала на момент добавления (2026-11-02). Печатается
	// в таблице состава: менеджер и заказчик сверяют позиции именно по нему,
	// а у материала их может быть несколько — в позицию идёт тот, что был в
	// таблице менеджера, иначе первый по справочнику.
	EKN   string  `json:"ekn,omitempty"`
	Name  string  `json:"name"`
	Unit  string  `json:"unit"`
	Qty   float64 `json:"qty"`
	Price float64 `json:"price"`
	Sum   float64 `json:"sum"`
	// PhotoKey/ImageName — картинка материала из карточки Базы знаний
	// (2026-11-05). Снимок, как и цена: в документе должна быть картинка того
	// материала, который был в предложении на момент сборки, а карточку потом
	// могут переписать. Пусто — картинки нет, документ собирается без неё.
	PhotoKey  string `json:"photoKey,omitempty"`
	ImageName string `json:"imageName,omitempty"`
}

// OfferClient — данные клиента в предложении.
type OfferClient struct {
	Name    string `json:"name"`
	Contact string `json:"contact"`
}

// OfferSystem — система, которую менеджер выбрал для предложения.
//
// Хранится и код, и заметка (2026-11-03): у живых карточек `system_sku` не
// заполнен, и без номера заметки система не находится после перезапуска.
//
// Позиции выбор системы НЕ добавляет: цены и состав ведёт менеджер.
type OfferSystem struct {
	SKU    string `json:"sku,omitempty"`
	NoteID int64  `json:"noteId,omitempty"`
}

// Empty — система не выбрана.
func (s OfferSystem) Empty() bool {
	return s.SKU == "" && s.NoteID <= 0
}

// Same — тот же выбор. Код главнее заметки и сравнивается без учёта регистра:
// в карточку его пишут руками, и `TN-roof` с пробелом — тот же код.
func (s OfferSystem) Same(other OfferSystem) bool {
	a := strings.ToUpper(strings.TrimSpace(s.SKU))
	b := strings.ToUpper(strings.TrimSpace(other.SKU))
	if a != "" || b != "" {
		return a != "" && a == b
	}
	return s.NoteID > 0 && s.NoteID == other.NoteID
}

// OfferSystems — системы предложения в порядке выбора.
//
// Список, а не одно значение (2026-11-05): менеджеру нужно предложить и
// сравнить несколько систем в одном КП — как СТИЛОБАТ V1.0 и V2.0 в образце.
// Старые предложения хранят выбор объектом (`{"sku":…}` или `{}`); такой JSON
// читается UnmarshalJSON ниже, поэтому миграция данных не нужна.
type OfferSystems []OfferSystem

// Add дописывает систему, если её ещё нет. Порядок выбора сохраняется: по нему
// нумеруются разделы документа, и перескакивать при каждом сохранении нельзя.
func (l OfferSystems) Add(s OfferSystem) OfferSystems {
	if s.Empty() {
		return l
	}
	for _, existing := range l {
		if existing.Same(s) {
			return l
		}
	}
	return append(l, s)
}

// Remove убирает систему; нет её — список не меняется.
func (l OfferSystems) Remove(s OfferSystem) OfferSystems {
	out := make(OfferSystems, 0, len(l))
	for _, existing := range l {
		if existing.Same(s) {
			continue
		}
		out = append(out, existing)
	}
	return out
}

// UnmarshalJSON принимает и массив, и старый объект.
//
// Терпимость к старому виду — не удобство, а условие: `system_choice` живых КП
// записан объектом, и без этого разбора все уже выбранные системы разом
// пропали бы из документов.
func (l *OfferSystems) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" || trimmed == `""` {
		*l = OfferSystems{}
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var list []OfferSystem
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		if list == nil {
			list = []OfferSystem{}
		}
		*l = list
		return nil
	}
	if trimmed == `{}` {
		*l = OfferSystems{}
		return nil
	}
	var one OfferSystem
	if err := json.Unmarshal(data, &one); err != nil {
		return err
	}
	if one.Empty() {
		*l = OfferSystems{}
		return nil
	}
	*l = OfferSystems{one}
	return nil
}

// MarshalJSON пишет массив и никогда не пишет `null`: клиент, получивший
// null, снова увидит непривычный тип.
func (l OfferSystems) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]OfferSystem(l))
}

// Offer — коммерческое предложение.
type Offer struct {
	ID         int64             `json:"id"`
	Number     string            `json:"number"`
	OwnerEmail string            `json:"ownerEmail"`
	Client     OfferClient       `json:"client"`
	Items      []OfferItem       `json:"items"`
	FormValues map[string]string `json:"formValues"`
	// Systems — выбранные системы (2026-11-05). Пусто — ни одна не выбрана.
	// Поле `system` в JSON оставлено на чтение старых КП, см. Systems().
	Systems   OfferSystems `json:"systems"`
	Total     float64      `json:"total"`
	Status    string       `json:"status"`
	DocxKey   string       `json:"docxKey"`
	CreatedAt string       `json:"createdAt"`
	UpdatedAt string       `json:"updatedAt"`
}
