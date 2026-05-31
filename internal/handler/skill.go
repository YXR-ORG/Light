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

// ImportSkillZip receives raw ZIP bytes from the frontend, parses SKILL.md files,
// and saves each skill to the database. Returns the list of imported skills.
func (h *SkillHandler) ImportSkillZip(data []byte) ([]storage.Skill, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip: %w", err)
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

		skill, err := parseSkillMD(buf.String())
		if err != nil || skill.Name == "" {
			continue
		}
		if err := storage.SaveSkill(skill); err != nil {
			return nil, fmt.Errorf("save skill %q: %w", skill.Name, err)
		}
		imported = append(imported, *skill)
	}
	if len(imported) == 0 {
		return nil, fmt.Errorf("zip 中未找到有效的 SKILL.md 文件")
	}
	return imported, nil
}

// frontMatter is the YAML header of a SKILL.md file.
type frontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parseSkillMD parses a SKILL.md file into a Skill struct.
// Format:
//
//	---
//	name: my-skill
//	description: "..."
//	---
//	<markdown body>
func parseSkillMD(content string) (*storage.Skill, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		// No frontmatter — use filename as name, whole content as body
		return &storage.Skill{Content: content}, nil
	}
	// Find closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return &storage.Skill{Content: content}, nil
	}
	yamlPart := strings.TrimSpace(rest[:idx])
	body := strings.TrimSpace(rest[idx+4:])

	var fm frontMatter
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return nil, err
	}
	return &storage.Skill{
		Name:        fm.Name,
		Description: fm.Description,
		Content:     body,
	}, nil
}
