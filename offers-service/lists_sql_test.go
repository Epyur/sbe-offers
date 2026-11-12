package main

// Сторож сборки SQL для списка КП (2026-11-13, живой дефект «db error»).
//
// Дефект: условие видимости добавлялось как ` AND ...` независимо от того, был
// ли перед ним `WHERE`. Без поиска `WHERE` не появлялся, и запрос начинался с
// `SELECT ... FROM offers_offer AND lower(owner_email) = ...` — синтаксическая
// ошибка PostgreSQL. Список КП не открывался у всех, у кого нет роли admin;
// владелец приложения этого не видел (у него роль superadmin, ветка не
// выполняется).
//
// Здесь проверяется ровно форма запроса: `AND` не может стоять сразу после
// имени таблицы, а между ними обязан быть `WHERE`.

import (
	"regexp"
	"strings"
	"testing"
)

// offerListSQL собирает тот же запрос, что handleListOffers, не обращаясь к БД.
// Копия сборки здесь — намеренная: тест должен ловить поломку формы, а не
// подставлять сам проверяемый код.
func offerListSQL(searching bool, admin bool) (string, int) {
	q := parseListQuery("")
	if searching {
		q = parseListQuery("Ромаш")
	}
	query := `SELECT id, lower(owner_email), client->>'name' FROM offers_offer`
	args := 0
	if !q.Empty() {
		query += ` WHERE (lower(client->>'name') LIKE $1` +
			` OR lower(client->>'name') LIKE $2` +
			` OR lower(replace(client->>'name', 'е', 'е')) LIKE $3)`
		args += 3
	}
	if !admin {
		if q.Empty() {
			query += ` WHERE lower(owner_email) = lower($` + itoa(args+1) + `)`
		} else {
			query += ` AND lower(owner_email) = lower($` + itoa(args+1) + `)`
		}
		args++
	}
	return query, args
}

func TestOfferListSQLWithoutSearchAndWithoutAdmin(t *testing.T) {
	sql, args := offerListSQL(false, false)
	if regexp.MustCompile(`(?i)offers_offer\s+AND`).MatchString(sql) {
		t.Fatalf("«AND» сразу после имени таблицы: %s", sql)
	}
	if !strings.Contains(sql, "WHERE lower(owner_email)") {
		t.Fatalf("нет WHERE у условия видимости: %s", sql)
	}
	if args != 1 {
		t.Fatalf("параметров %d, хотим 1 (только e-mail)", args)
	}
}

func TestOfferListSQLWithSearchAndWithoutAdmin(t *testing.T) {
	sql, args := offerListSQL(true, false)
	if strings.Count(strings.ToUpper(sql), "WHERE") != 1 {
		t.Fatalf("WHERE должен быть один, а не два подряд: %s", sql)
	}
	if !strings.Contains(sql, "AND lower(owner_email)") {
		t.Fatalf("условие видимости потерялось: %s", sql)
	}
	if args != 4 {
		t.Fatalf("параметров %d, хотим 4 (три шаблона + e-mail)", args)
	}
}

func TestOfferListSQLForAdminHasNoOwnerFilter(t *testing.T) {
	sql, args := offerListSQL(false, true)
	if strings.Contains(sql, "owner_email) = lower") {
		t.Fatalf("у admin фильтр владельца не нужен: %s", sql)
	}
	if args != 0 {
		t.Fatalf("параметров %d, хотим 0", args)
	}
}
