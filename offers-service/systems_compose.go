package main

import (
	"net/http"
	"strings"
)

// Состав системы в существующее предложение (Task 7, 2026-10-31).
//
// «Добавить систему» в редакторе КП не может быть клиентской склейкой: цену
// позиции клиент не знает (её нет ни у системы, ни у материала), а сервер
// знает вилку, которую видел в таблицах менеджера. Поэтому состав собирает
// сервер и дописывает его в сохранённое предложение.
//
// Что НЕ делается и почему:
//   - цена не выдумывается: берётся нижняя граница вилки; если вилки нет —
//     ноль, и материал называется в ответе, чтобы менеджер увидел «цену надо
//     загрузить таблицей». Подставить правдоподобное число тут хуже всего;
//   - позиции ДОПИСЫВАЮТСЯ, а не заменяют состав: система — это набор, который
//     человек захотел добавить к уже собранному КП.

// ComposeSystemResponse — что добавилось и что осталось без цены.
type ComposeSystemResponse struct {
	Offer Offer `json:"offer"`
	// Added — сколько позиций дописано.
	Added int `json:"added"`
	// MissingPrice — материалы, у которых цены ещё не видели: их позиции
	// попадут в КП с нулём, и менеджер должен об этом знать.
	MissingPrice []string `json:"missingPrice,omitempty"`
	// Problems — строки состава, которые дописать не удалось.
	Problems []string `json:"problems,omitempty"`
}

// itemForMaterial — позиция КП из материала. Второе значение — «цена
// известна»: вилка есть, значит видели цены в таблицах менеджера.
//
// Цена позиции — нижняя граница вилки. Верхняя была бы осторожнее для
// заказчика, но лгать в другую сторону так же нехорошо: менеджер увидит цену,
// поправит руками, а КП с нулём или пустым полем он замечает сразу.
func itemForMaterial(m Material) (OfferItem, bool) {
	id := m.ID
	price := 0.0
	hasPrice := m.PriceMin != nil
	if hasPrice {
		price = *m.PriceMin
	}
	return OfferItem{
		CatalogID: &id,
		Name:      m.Name,
		Unit:      m.Unit,
		Qty:       1,
		Price:     price,
		Sum:       roundMoney(price),
	}, hasPrice
}

// appendComposition дописывает позиции состава и возвращает число добавленных.
//
// Несвязанные строки не теряются: они уходят в Problems, потому что человек,
// добавивший систему, должен увидеть, чего в ней не хватает, а не считать её
// полной. Материалы без вилки цен называются отдельно.
func appendComposition(
	items []OfferItem,
	links []CompositionItem,
	byID map[int64]Material,
	resp *ComposeSystemResponse,
) ([]OfferItem, int) {
	added := 0
	for _, link := range links {
		material, ok := byID[link.MaterialID]
		if !link.Found || !ok {
			resp.Problems = append(resp.Problems, link.Raw)
			continue
		}
		item, hasPrice := itemForMaterial(material)
		if !hasPrice {
			resp.MissingPrice = append(resp.MissingPrice, material.Name)
		}
		items = append(items, item)
		added++
	}
	return items, added
}

// FindSystem — система по коду в списке из Базы знаний.
//
// Код сравнивается без учёта регистра и краевых пробелов: в карточку его пишут
// руками, и `TN-roof-classic` с пробелом на конце — тот же код.
func FindSystem(systems []SystemCard, sku string) (SystemCard, bool) {
	want := strings.ToUpper(strings.TrimSpace(sku))
	if want == "" {
		return SystemCard{}, false
	}
	for _, card := range systems {
		if strings.ToUpper(strings.TrimSpace(card.SKU)) == want {
			return card, true
		}
	}
	return SystemCard{}, false
}

// handleAddSystem дописывает состав системы в предложение.
//
// Система ищется в живой ленте Базы знаний, а не в кеше карточек: человек
// выбрал её из раздела «Системы», который читает базу знаний вживую, и кеш
// мог отстать от того, что он видел на экране.
func (s *Server) handleAddSystem(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	var req struct {
		SKU string `json:"sku"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	materials, err := s.listMaterials(r.Context(), false)
	if err != nil {
		dbError(w, "add system: materials", err)
		return
	}
	systems, err := SystemsFromKB(r.Context(), s.kb, materials)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "База знаний недоступна: " + err.Error(),
		})
		return
	}
	card, found := FindSystem(systems, req.SKU)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "система не найдена"})
		return
	}

	// Вилка цен по материалам: одна выборка вместо запроса на каждую строку.
	byID := make(map[int64]Material, len(materials))
	for _, m := range materials {
		byID[m.ID] = m
	}

	resp := ComposeSystemResponse{Problems: []string{}}
	offer.Items, resp.Added = appendComposition(offer.Items, card.CompositionLinks, byID, &resp)
	if resp.Added == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "в составе системы нет ни одного материала из справочника",
		})
		return
	}
	if err := s.persistOfferItems(r.Context(), offer); err != nil {
		dbError(w, "add system: save offer", err)
		return
	}
	resp.Offer = offer
	writeJSON(w, http.StatusOK, resp)
}
