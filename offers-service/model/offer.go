// Package model — данные предметной области, общие для сервиса, скилов и
// чистых модулей. Отдельный пакет, а не файлы в main: main нельзя
// импортировать, и промпты/чек-листы без этого не проверяются тестами.
package model

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
}

// OfferClient — данные клиента в предложении.
type OfferClient struct {
	Name    string `json:"name"`
	Contact string `json:"contact"`
}

// OfferSystem — система, которую менеджер выбрал для предложения.
//
// Хранится и код, и заметка (2026-11-03): у живых карточек `system_sku` не
// заполнен, и без номера заметки система не находится после перезапуска. Поле
// одно на предложение: система в КП одна, а описание, преимущества и ссылки
// печатаются из её карточки.
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

// Offer — коммерческое предложение.
type Offer struct {
	ID         int64             `json:"id"`
	Number     string            `json:"number"`
	OwnerEmail string            `json:"ownerEmail"`
	Client     OfferClient       `json:"client"`
	Items      []OfferItem       `json:"items"`
	FormValues map[string]string `json:"formValues"`
	// System — выбранная система (2026-11-03). Пусто — система не выбрана.
	System    OfferSystem `json:"system"`
	Total     float64     `json:"total"`
	Status    string      `json:"status"`
	DocxKey   string      `json:"docxKey"`
	CreatedAt string      `json:"createdAt"`
	UpdatedAt string      `json:"updatedAt"`
}
