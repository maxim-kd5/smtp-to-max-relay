package acl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleAliasAdmin Role = "alias_admin"
	RoleDLQAdmin   Role = "dlq_admin"
	RoleStatsView  Role = "stats_viewer"
)

var roleOrder = []Role{RoleSuperAdmin, RoleAliasAdmin, RoleDLQAdmin, RoleStatsView}

type roleMembers struct {
	Users map[int64]struct{}
	Chats map[int64]struct{}
}

type fileModel struct {
	Roles map[string]fileRole `json:"roles"`
}

type fileRole struct {
	Users []int64 `json:"users,omitempty"`
	Chats []int64 `json:"chats,omitempty"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	roles map[Role]*roleMembers
}

func Open(path string, bootstrapUsers, bootstrapChats []int64) (*Store, error) {
	s := &Store{path: path, roles: defaultRoles()}
	if err := s.load(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range bootstrapUsers {
		if id > 0 {
			s.roles[RoleSuperAdmin].Users[id] = struct{}{}
		}
	}
	for _, id := range bootstrapChats {
		if id != 0 {
			s.roles[RoleSuperAdmin].Chats[id] = struct{}{}
		}
	}
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

func defaultRoles() map[Role]*roleMembers {
	roles := map[Role]*roleMembers{}
	for _, role := range roleOrder {
		roles[role] = &roleMembers{Users: map[int64]struct{}{}, Chats: map[int64]struct{}{}}
	}
	return roles
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var m fileModel
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for k, r := range m.Roles {
		role, err := parseRole(k)
		if err != nil {
			continue
		}
		for _, id := range r.Users {
			if id > 0 {
				s.roles[role].Users[id] = struct{}{}
			}
		}
		for _, id := range r.Chats {
			if id != 0 {
				s.roles[role].Chats[id] = struct{}{}
			}
		}
	}
	return nil
}

func (s *Store) IsSuperAdmin(userID, chatID int64) bool {
	return s.hasRole(RoleSuperAdmin, userID, chatID)
}
func (s *Store) CanManageAliases(userID, chatID int64) bool {
	return s.hasRole(RoleAliasAdmin, userID, chatID)
}
func (s *Store) CanReplayDLQ(userID, chatID int64) bool {
	return s.hasRole(RoleDLQAdmin, userID, chatID)
}
func (s *Store) CanViewStats(userID, chatID int64) bool {
	return s.hasRole(RoleStatsView, userID, chatID)
}

func (s *Store) hasRole(role Role, userID, chatID int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if userID > 0 {
		if _, ok := s.roles[RoleSuperAdmin].Users[userID]; ok {
			return true
		}
		if _, ok := s.roles[role].Users[userID]; ok {
			return true
		}
	}
	if chatID != 0 {
		if _, ok := s.roles[RoleSuperAdmin].Chats[chatID]; ok {
			return true
		}
		if _, ok := s.roles[role].Chats[chatID]; ok {
			return true
		}
	}
	return false
}

func (s *Store) Grant(roleName, scope string, id int64) error {
	role, err := parseRole(roleName)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := addMember(s.roles[role], scope, id); err != nil {
		return err
	}
	return s.persistLocked()
}

func (s *Store) Revoke(roleName, scope string, id int64) error {
	role, err := parseRole(roleName)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := removeMember(s.roles[role], scope, id); err != nil {
		return err
	}
	return s.persistLocked()
}

func addMember(m *roleMembers, scope string, id int64) error {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "user":
		if id <= 0 {
			return fmt.Errorf("user id must be > 0")
		}
		m.Users[id] = struct{}{}
		return nil
	case "chat":
		if id == 0 {
			return fmt.Errorf("chat id must not be 0")
		}
		m.Chats[id] = struct{}{}
		return nil
	default:
		return fmt.Errorf("scope must be user|chat")
	}
}

func removeMember(m *roleMembers, scope string, id int64) error {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "user":
		delete(m.Users, id)
		return nil
	case "chat":
		delete(m.Chats, id)
		return nil
	default:
		return fmt.Errorf("scope must be user|chat")
	}
}

func parseRole(name string) (Role, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "super_admin", "superadmin":
		return RoleSuperAdmin, nil
	case "alias_admin", "aliases":
		return RoleAliasAdmin, nil
	case "dlq_admin", "dlq":
		return RoleDLQAdmin, nil
	case "stats_viewer", "stats":
		return RoleStatsView, nil
	default:
		return "", fmt.Errorf("unknown role: %s", name)
	}
}

func (s *Store) WhoAmI(userID, chatID int64) string {
	roles := make([]string, 0)
	for _, role := range roleOrder {
		if s.hasRole(role, userID, chatID) {
			roles = append(roles, string(role))
		}
	}
	if len(roles) == 0 {
		roles = append(roles, "none")
	}
	return fmt.Sprintf("user_id=%d chat_id=%d roles=%s", userID, chatID, strings.Join(roles, ","))
}

func (s *Store) SuperAdminChatIDs() []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]int64, 0, len(s.roles[RoleSuperAdmin].Chats))
	for id := range s.roles[RoleSuperAdmin].Chats {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Store) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	model := fileModel{Roles: map[string]fileRole{}}
	for _, role := range roleOrder {
		rm := s.roles[role]
		fr := fileRole{Users: keys(rm.Users), Chats: keys(rm.Chats)}
		if len(fr.Users) == 0 && len(fr.Chats) == 0 {
			continue
		}
		model.Roles[string(role)] = fr
	}
	data, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func keys(m map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func ParseIDsCSV(v string) ([]int64, error) {
	parts := strings.Split(v, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}
