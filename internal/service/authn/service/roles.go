package service

import "strings"

// adminRule — есть ли в claims роль админа (AUTH_ADMIN_ROLE). Правило — как у метода jwt в ruto:
//   - путь задан (AUTH_ROLES_CLAIM) — роль ищется только там, как есть;
//   - "клиент:роль" — только у этого клиента, resource_access.<клиент>.roles;
//   - просто "роль" — в roles, role, realm_access.roles и у любого клиента в resource_access.
type adminRule struct {
	role   string
	client string
	path   string
}

func newAdminRule(role, rolesClaim string) adminRule {
	r := adminRule{role: strings.TrimSpace(role), path: strings.TrimSpace(rolesClaim)}
	if r.path == "" {
		if client, clientRole, ok := strings.Cut(r.role, ":"); ok && client != "" && clientRole != "" {
			r.client, r.role = client, clientRole
		}
	}
	return r
}

func (r adminRule) matches(claims map[string]any) bool {
	if r.role == "" {
		return false
	}
	is := func(v string) bool { return v == r.role }

	if r.path != "" {
		return visitRoles(claimByPath(claims, r.path), is)
	}

	if r.client != "" {
		return visitRoles(claimByPath(claims, "resource_access."+r.client+".roles"), is)
	}

	if visitRoles(claims["roles"], is) || visitRoles(claims["role"], is) ||
		visitRoles(claimByPath(claims, "realm_access.roles"), is) {
		return true
	}
	resourceAccess, _ := claims["resource_access"].(map[string]any)
	for _, clientAccess := range resourceAccess {
		if access, ok := clientAccess.(map[string]any); ok && visitRoles(access["roles"], is) {
			return true
		}
	}
	return false
}

// claimByPath — значение по пути через точку. Сначала путь целиком как имя claim: у Auth0 имя само
// с точками (https://example.kz/roles).
func claimByPath(claims map[string]any, path string) any {
	if v, ok := claims[path]; ok {
		return v
	}

	var cur any = claims
	for part := range strings.SplitSeq(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		if cur, ok = m[part]; !ok {
			return nil
		}
	}
	return cur
}

// visitRoles — роли в значении claim: список строк, строка через пробел или объект, где роли —
// ключи (Zitadel: {"admin": {...}})
func visitRoles(value any, fn func(role string) bool) bool {
	switch typed := value.(type) {
	case string:
		for v := range strings.FieldsSeq(typed) {
			if fn(v) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if v, ok := item.(string); ok && fn(v) {
				return true
			}
		}
	case []string:
		for _, v := range typed {
			if fn(v) {
				return true
			}
		}
	case map[string]any:
		for v := range typed {
			if fn(v) {
				return true
			}
		}
	}
	return false
}
