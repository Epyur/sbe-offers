// Package model — данные предметной области, общие для сервиса, скилов и
// чистых модулей. Отдельный пакет, а не файлы в main: main нельзя
// импортировать, и промпты/чек-листы без этого не проверяются тестами.
package model

import (
	"encoding/json"
	"sort"
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

// Ref — выбор карточки Базы знаний: система или материал (2026-11-13).
//
// Раньше это был OfferSystem. Система и материал выбираются ОДИНАКОВО — код из
// карточки и/или номер заметки, — и второй тип с теми же полями и той же
// логикой слияния завёл бы вторую копию правила (ошибка модели, правило
// 2026-11-11). Поэтому один тип и два именованных списка: OfferSystems и
// OfferMaterials.
//
// Хранится и код, и заметка (2026-11-03): у живых карточек `system_sku` не
// заполнен, и без номера заметки выбор не находится после перезапуска.
type Ref struct {
	SKU    string `json:"sku,omitempty"`
	NoteID int64  `json:"noteId,omitempty"`
}

// OfferSystem — старое имя выбора; оставлено, чтобы не переписывать вызовы.
type OfferSystem = Ref

// Empty — карточка не выбрана.
func (r Ref) Empty() bool {
	return r.SKU == "" && r.NoteID <= 0
}

// Same — тот же выбор. Код главнее заметки и сравнивается без учёта регистра:
// в карточку его пишут руками, и `TN-roof` с пробелом — тот же код.
func (r Ref) Same(other Ref) bool {
	a := strings.ToUpper(strings.TrimSpace(r.SKU))
	b := strings.ToUpper(strings.TrimSpace(other.SKU))
	if a != "" || b != "" {
		return a != "" && a == b
	}
	return r.NoteID > 0 && r.NoteID == other.NoteID
}

// RefList — общий список выбора: Add/Remove/Marshal/Unmarshal и терпимость к
// старому виду (объект вместо массива) для систем и материалов одинаковы.
type RefList []Ref

// Add дописывает карточку, если её ещё нет. Порядок выбора сохраняется: по
// нему нумеруются разделы документа, и перескакивать при каждом сохранении
// нельзя.
func (l RefList) Add(r Ref) RefList {
	if r.Empty() {
		return l
	}
	for _, existing := range l {
		if existing.Same(r) {
			return l
		}
	}
	return append(l, r)
}

// Remove убирает карточку; нет её — список не меняется.
func (l RefList) Remove(r Ref) RefList {
	out := make(RefList, 0, len(l))
	for _, existing := range l {
		if existing.Same(r) {
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
func (l *RefList) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" || trimmed == `""` {
		*l = RefList{}
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var list []Ref
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		if list == nil {
			list = []Ref{}
		}
		*l = list
		return nil
	}
	if trimmed == `{}` {
		*l = RefList{}
		return nil
	}
	var one Ref
	if err := json.Unmarshal(data, &one); err != nil {
		return err
	}
	if one.Empty() {
		*l = RefList{}
		return nil
	}
	*l = RefList{one}
	return nil
}

// MarshalJSON пишет массив и никогда не пишет `null`: клиент, получивший
// null, снова увидит непривычный тип.
func (l RefList) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]Ref(l))
}

// OfferSystems — системы предложения в порядке выбора.
//
// Список, а не одно значение (2026-11-05): менеджеру нужно предложить и
// сравнить несколько систем в одном КП — как СТИЛОБАТ V1.0 и V2.0 в образце.
// Старые предложения хранят выбор объектом (`{"sku":…}` или `{}`); такой JSON
// читается UnmarshalJSON выше, поэтому миграция данных не нужна.
type OfferSystems = RefList

// OfferMaterials — материалы, предложенные описанием (2026-11-13).
//
// Материал, как и система, — ОБЪЕКТ предложения: в документ попадает его
// описание и ссылки, а не цена. Позицией он становится только через выбор в
// таблице состава — это другой путь и остаётся как был.
type OfferMaterials = RefList

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
	Systems OfferSystems `json:"systems"`
	// Materials — материалы, предложенные описанием (2026-11-13). Позиций они
	// не добавляют: цена строки приходит из таблицы менеджера.
	Materials OfferMaterials `json:"materials"`
	Total     float64        `json:"total"`
	Status    string         `json:"status"`
	DocxKey   string         `json:"docxKey"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
}

// SelectionHandle — устойчивый признак набора выбранных карточек: по нему
// ключуется кэш текстов, которые зависят от ВСЕГО набора.
func SelectionHandle(refs RefList) string {
	if len(refs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		if sku := strings.ToUpper(strings.TrimSpace(r.SKU)); sku != "" {
			parts = append(parts, "sku:"+sku)
			continue
		}
		if r.NoteID > 0 {
			parts = append(parts, "note:"+itoa64(r.NoteID))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// itoa64 — номер заметки строкой без пакета strconv на каждый вызов.
func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
