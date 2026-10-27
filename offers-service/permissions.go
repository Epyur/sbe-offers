package main

import (
	"log"
	"net/http"
	"sort"
	"strings"
)

// handleMyPermission возвращает роль текущего пользователя (по JWT email).
func (s *Server) handleMyPermission(w http.ResponseWriter, r *http.Request) {
	email, ok := r.Context().Value(permEmailCtx{}).(string)
	if !ok || email == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	role, err := s.effectiveRole(r.Context(), appIDFromEnv(), email)
	if err != nil {
		dbError(w, "запрос", err)
		return
	}
	if role == "" {
		writeJSON(w, http.StatusOK, map[string]any{"email": email, "role": "", "hasAccess": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": email, "role": role, "hasAccess": true})
}

// handleListPermissions возвращает все глобальные роли (для admin). Источник
// правды — auth-service (см. authperms.go).
func (s *Server) handleListPermissions(w http.ResponseWriter, r *http.Request) {
	snap := s.permissionsSnapshot(r.Context())
	type perm struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	perms := make([]perm, 0, len(snap.Roles))
	for email, role := range snap.Roles {
		perms = append(perms, perm{Email: email, Role: role})
	}
	sort.Slice(perms, func(i, j int) bool { return perms[i].Email < perms[j].Email })
	writeJSON(w, http.StatusOK, map[string]any{"permissions": perms})
}

// handleSetPermission устанавливает роль ({email, role}); role="" — удаляет доступ.
func (s *Server) handleSetPermission(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Role = strings.TrimSpace(req.Role)
	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email is required"})
		return
	}
	if req.Role != "" && req.Role != "viewer" && req.Role != "editor" && req.Role != "admin" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "role must be viewer, editor or admin"})
		return
	}

	if req.Role == "" || req.Role != "admin" {
		if req.Email == ownerEmailFromEnv() {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "нельзя отозвать доступ владельца"})
			return
		}
	}

	if err := setPermissionUpstream(r.Context(), actorEmailFromCtx(r), req.Email, req.Role); err != nil {
		log.Printf("set permission upstream: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "не удалось сохранить права: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// actorEmailFromCtx — email вызывающего из контекста запроса (положен requirePerm).
func actorEmailFromCtx(r *http.Request) string {
	if v, ok := r.Context().Value(permEmailCtx{}).(string); ok {
		return v
	}
	return ""
}
