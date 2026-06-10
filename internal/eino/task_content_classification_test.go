package eino

import "testing"

func TestShouldRollbackTaskContentKeepsLongSubstantiveContent(t *testing.T) {
	longAnswer := make([]byte, taskContentRollbackMaxLen+1)
	for i := range longAnswer {
		longAnswer[i] = 'x'
	}

	if shouldRollbackTaskContent(string(longAnswer), true) {
		t.Fatal("long assistant content with a tool call should remain final content")
	}
}

func TestShouldRollbackTaskContentRollsBackShortToolNarration(t *testing.T) {
	if !shouldRollbackTaskContent("我先更新一下计划。", true) {
		t.Fatal("short assistant narration with a tool call should be rolled back")
	}
}

func TestShouldRollbackTaskContentKeepsContentWithoutToolCall(t *testing.T) {
	if shouldRollbackTaskContent("最终答案", false) {
		t.Fatal("assistant content without a tool call should not be rolled back")
	}
}
