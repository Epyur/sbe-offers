// Package model — данные предметной области, общие для сервиса, скилов и
// чистых модулей. Отдельный пакет, а не файлы в main: main нельзя
// импортировать, и промпты/чек-листы без этого не проверяются тестами.
package model

// OfferItem — позиция предложения. Это СНИМОК: цена и сумма фиксируются в
// момент добавления карточки, поэтому последующая правка каталога на старые
// предложения не влияет.
type OfferItem struct {
	CatalogID *int64  `json:"catalogId,omitempty"`
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	Qty       float64 `json:"qty"`
	Price     float64 `json:"price"`
	Sum       float64 `json:"sum"`
}

// OfferClient — данные клиента в предложении.
type OfferClient struct {
	Name    string `json:"name"`
	Contact string `json:"contact"`
}

// Offer — коммерческое предложение.
type Offer struct {
	ID         int64             `json:"id"`
	Number     string            `json:"number"`
	OwnerEmail string            `json:"ownerEmail"`
	Client     OfferClient       `json:"client"`
	Items      []OfferItem       `json:"items"`
	FormValues map[string]string `json:"formValues"`
	Total      float64           `json:"total"`
	Status     string            `json:"status"`
	DocxKey    string            `json:"docxKey"`
	CreatedAt  string            `json:"createdAt"`
	UpdatedAt  string            `json:"updatedAt"`
}
