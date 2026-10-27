package main

import "testing"

func TestRoleRank(t *testing.T) {
	if roleRank("admin") <= roleRank("editor") {
		t.Fatalf("admin должен быть выше editor")
	}
	if roleRank("editor") <= roleRank("viewer") {
		t.Fatalf("editor должен быть выше viewer")
	}
	if roleRank("viewer") <= roleRank("") {
		t.Fatalf("viewer должен быть выше отсутствия роли")
	}
	if roleRank("несуществующая") != 0 {
		t.Fatalf("неизвестная роль должна быть равна нулю")
	}
}

func TestRoleFromSnapshotPrefersPersonalRole(t *testing.T) {
	snap := permSnapshot{
		Roles:           map[string]string{"ivanov@tn.ru": "editor"},
		CommonAccess:    "viewer",
		CommonAccessSet: true,
	}
	if got := roleFromSnapshot(snap, "IVANOV@tn.ru", ""); got != "editor" {
		t.Fatalf("ожидали персональную роль editor, получили %q", got)
	}
	if got := roleFromSnapshot(snap, "petrov@tn.ru", ""); got != "viewer" {
		t.Fatalf("ожидали общий доступ viewer, получили %q", got)
	}
}

func TestRoleFromSnapshotNoCommonAccessMeansNoAccess(t *testing.T) {
	snap := permSnapshot{Roles: map[string]string{}, CommonAccess: "", CommonAccessSet: true}
	if got := roleFromSnapshot(snap, "petrov@tn.ru", "viewer"); got != "" {
		t.Fatalf("пустой общий доступ — это «доступа нет», получили %q", got)
	}
}
