package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// Состав системы в предложение (Task 7, 2026-10-31).
//
// «Добавить систему» не может быть клиентской склейкой: цену позиции клиент не
// знает (её нет ни у системы, ни у материала), а сервер знает вилку, которую
// видел в таблицах менеджера. Поэтому состав собирает сервер и дописывает его
// в сохранённое предложение.
//
// Правило у плагина и у бота ОДНО — assembleSystem; разница только во входе
// (JWT против секрета сервиса с chat_id) и в том, кто отвечает за ошибку.

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

// composeError — отказ сборки, который вызывающий превращает в HTTP-статус.
type composeError struct {
	status int
	text   string
}

func (e composeError) Error() string { return e.text }

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

// assembleSystem дописывает состав системы в предложение.
//
// Система ищется в живой ленте Базы знаний, а не в кеше карточек: человек
// выбрал её из раздела «Системы», который читает базу знаний вживую, и кеш
// мог отстать от того, что он видел на экране.
//
// Возвращает обновлённое предложение и сводку. Если дописать нечего —
// composeError со статусом 400, и предложение НЕ сохраняется.
func (s *Server) assembleSystem(ctx context.Context, offer Offer, sku string) (Offer, ComposeSystemResponse, error) {
	materials, err := s.listMaterials(ctx, false)
	if err != nil {
		return offer, ComposeSystemResponse{}, composeError{
			status: http.StatusInternalServerError, text: "не удалось прочитать материалы",
		}
	}
	systems, err := SystemsFromKB(ctx, s.kb, materials)
	if err != nil {
		return offer, ComposeSystemResponse{}, composeError{
			status: http.StatusBadGateway, text: "База знаний недоступна: " + err.Error(),
		}
	}
	card, found := FindSystem(systems, sku)
	if !found {
		return offer, ComposeSystemResponse{}, composeError{
			status: http.StatusNotFound, text: "система не найдена",
		}
	}

	// Вилка цен по материалам: одна выборка вместо запроса на каждую строку.
	byID := make(map[int64]Material, len(materials))
	for _, m := range materials {
		byID[m.ID] = m
	}

	resp := ComposeSystemResponse{Problems: []string{}}
	offer.Items, resp.Added = appendComposition(offer.Items, card.CompositionLinks, byID, &resp)
	if resp.Added == 0 {
		return offer, resp, composeError{
			status: http.StatusBadRequest,
			text:   "в составе системы нет ни одного материала из справочника",
		}
	}
	if err := s.persistOfferItems(ctx, offer); err != nil {
		return offer, resp, composeError{
			status: http.StatusInternalServerError, text: "не удалось сохранить предложение",
		}
	}
	// Состав изменился — значит, текст блоков «по промту» про старый состав
	// больше не годится. Кэш стираем здесь, при записи, а не при печати:
	// между ними человек может открыть КП и увидеть старый текст (2026-11-01).
	offer.FormValues = dropStaleBlockTexts(offer.FormValues)
	if err := s.persistOfferFormValues(ctx, offer); err != nil {
		log.Printf("add system: тексты блоков не сброшены у КП %s: %v", offer.Number, err)
	}
	resp.Offer = offer
	return offer, resp, nil
}

// dropStaleBlockTexts убирает хеш данных из form_values: при следующей сборке
// документа тексты блоков будут написаны заново.
//
// Сам текст не стирается: человек мог видеть его в плагине, и тихо удалять
// текст, который он читал, хуже, чем переписать его при печати. Хеш — то, по
// чему решается «этот текст ещё про это предложение».
func dropStaleBlockTexts(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		if k == genKeySection {
			continue
		}
		out[k] = v
	}
	return out
}

// writeComposeError отвечает отказом сборки: текст ошибки — для человека.
func writeComposeError(w http.ResponseWriter, err error) {
	if ce, ok := err.(composeError); ok {
		if ce.status == http.StatusInternalServerError {
			dbError(w, "add system", fmt.Errorf("%s", ce.text))
			return
		}
		writeJSON(w, ce.status, map[string]any{"error": ce.text})
		return
	}
	dbError(w, "add system", err)
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

// handleAddSystem — «Добавить систему» в редакторе плагина.
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
	_, resp, err := s.assembleSystem(r.Context(), offer, req.SKU)
	if err != nil {
		writeComposeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
