package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"light-ai/internal/storage"
)

type SkillHandler struct{}

func NewSkillHandler() *SkillHandler { return &SkillHandler{} }

func (h *SkillHandler) ListSkills() ([]storage.Skill, error) {
	return storage.ListSkills()
}

func (h *SkillHandler) ToggleSkill(id string, enabled bool) error {
	return storage.ToggleSkill(id, enabled)
}

func (h *SkillHandler) DeleteSkill(id string) error {
	return storage.DeleteSkill(id)
}

func (h *SkillHandler) SaveSkill(s storage.Skill) (storage.Skill, error) {
	if err := storage.SaveSkill(&s); err != nil {
		return storage.Skill{}, err
	}
	return s, nil
}

// ImportSkillZip receives raw ZIP bytes and the original filename from the frontend,
// parses SKILL.md, and saves the skill to the database.
func (h *SkillHandler) ImportSkillZip(data []byte, zipName string) ([]storage.Skill, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip: %w", err)
	}

	// Derive a fallback name from the zip filename (strip extension)
	fallbackName := strings.TrimSuffix(zipName, ".zip")
	fallbackName = strings.TrimSuffix(fallbackName, ".ZIP")
	if fallbackName == "" {
		fallbackName = "未命名技能"
	}

	var imported []storage.Skill
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.ToUpper(filepath.Base(f.Name)) != "SKILL.MD" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		buf := new(bytes.Buffer)
		buf.ReadFrom(rc)
		rc.Close()

		skill := parseSkillMD(buf.String(), fallbackName)
		if err := storage.SaveSkill(skill); err != nil {
			return nil, fmt.Errorf("save skill %q: %w", skill.Name, err)
		}
		imported = append(imported, *skill)
	}
	if len(imported) == 0 {
		return nil, fmt.Errorf("zip 中未找到 SKILL.md 文件")
	}
	return imported, nil
}

// frontMatter is the optional YAML header of a SKILL.md file.
type frontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parseSkillMD parses a SKILL.md file.
// If YAML frontmatter (---) is present, name and description come from there.
// Otherwise, fallbackName is used as name, and the first non-empty line as description.
// The full content is always stored as-is for the LLM to use.
func parseSkillMD(content, fallbackName string) *storage.Skill {
	content = strings.TrimSpace(content)

	// Try YAML frontmatter
	if strings.HasPrefix(content, "---") {
		rest := content[3:]
		idx := strings.Index(rest, "\n---")
		if idx >= 0 {
			yamlPart := strings.TrimSpace(rest[:idx])
			var fm frontMatter
			if err := yaml.Unmarshal([]byte(yamlPart), &fm); err == nil && fm.Name != "" {
				return &storage.Skill{
					Name:        fm.Name,
					Description: fm.Description,
					Content:     content,
				}
			}
		}
	}

	// No frontmatter: use fallbackName, first non-empty line as description
	description := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		// Skip markdown headings and horizontal rules
		if line == "" || line == "---" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip bold/italic markers
		description = strings.NewReplacer("**", "", "*", "", "`", "").Replace(line)
		break
	}

	return &storage.Skill{
		Name:        fallbackName,
		Description: description,
		Content:     content,
	}
}
