package ragagent

import "strings"

func (s MemoryScope) normalized(defaultTenant string) MemoryScope {
	s.UserID = strings.TrimSpace(s.UserID)
	s.Tenant = strings.TrimSpace(s.Tenant)
	if s.Tenant == "" {
		s.Tenant = strings.TrimSpace(defaultTenant)
	}
	return s
}
