package eino

import (
	"context"
	"encoding/json"
	"testing"
	"light-ai/internal/kb"
)

// 验证 search_knowledge 工具在发生同义词归一化时，返回的 JSON 包含 note 提示
func TestNoteInToolResponse(t *testing.T) {
	dir := t.TempDir()
	s, err := kb.GetStore("test-kb", dir)
	if err != nil {
		t.Fatalf("GetStore: %v", err)
	}
	defer kb.CloseStore("test-kb")

	docID, _ := s.InsertDocument("test.txt", "text/plain", 100)
	s.InsertChunks(docID, []kb.Chunk{
		{Content: "孙小仙是公平国的少女，性格机敏勇敢。", ChunkIndex: 0},
	})
	if err := s.AddSynonym("孙彤", "孙小仙"); err != nil {
		t.Fatalf("AddSynonym: %v", err)
	}

	tool, err := NewKnowledgeSearchTool("test-kb", dir)
	if err != nil {
		t.Fatalf("NewKnowledgeSearchTool: %v", err)
	}

	out, err := tool.InvokableRun(context.Background(), `{"query":"孙彤","top_k":5}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	t.Logf("工具返回 JSON:\n%s", out)

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp["note"] == nil {
		t.Errorf("note 字段缺失！LLM 收不到归一化提示")
	}
	if resp["synonym_mappings"] == nil {
		t.Errorf("synonym_mappings 字段缺失")
	}
}
