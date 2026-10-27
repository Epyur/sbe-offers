package main

import "strings"

// prepareOffer — граница входа предложения: одна проверка и одна нормализация
// на все пути (плагин и бот). Внутри — доверяем данным (Boundary Discipline).
func prepareOffer(offer Offer) (Offer, string) {
	if msg := validateOffer(offer); msg != "" {
		return offer, msg
	}
	offer.Total = recalcOfferTotal(offer.Items)
	if offer.Status != "final" {
		offer.Status = "draft"
	}
	if offer.FormValues == nil {
		offer.FormValues = map[string]string{}
	}
	return offer, ""
}

// canManageOffer — можно ли править/удалять предложение: административный
// уровень (admin или глобальный superadmin) либо собственное предложение.
func canManageOffer(role, email string, offer Offer) bool {
	if isAdminRole(role) {
		return true
	}
	return strings.EqualFold(offer.OwnerEmail, email)
}
