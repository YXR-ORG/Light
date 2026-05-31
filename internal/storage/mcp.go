package storage

import "time"

func ListMCPServers() ([]MCPServer, error) {
	var servers []MCPServer
	result := DB.Order("created_at asc").Find(&servers)
	return servers, result.Error
}

func SaveMCPServer(s *MCPServer) error {
	now := time.Now().Format(time.RFC3339)
	if s.ID == "" {
		s.ID = NewID()
		s.CreatedAt = now
		s.UpdatedAt = now
		return DB.Create(s).Error
	}
	s.UpdatedAt = now
	return DB.Save(s).Error
}

func DeleteMCPServer(id string) error {
	return DB.Delete(&MCPServer{}, "id = ?", id).Error
}

func ToggleMCPServer(id string, enabled bool) error {
	return DB.Model(&MCPServer{}).Where("id = ?", id).Update("enabled", enabled).Error
}
