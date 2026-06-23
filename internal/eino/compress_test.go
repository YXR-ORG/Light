package eino

import (
	"path/filepath"
	"testing"
)

func TestIsSubPath_NormalChild(t *testing.T) {
	base := "/tmp/work"
	target := filepath.Join(base, "trunc", "call_123")
	if !isSubPath(target, base) {
		t.Fatalf("target %q should be inside base %q", target, base)
	}
}

func TestIsSubPath_PathTraversalWithDotDot(t *testing.T) {
	base := "/tmp/work/.light/reduction"
	target := filepath.Join(base, "..", "..", "..", "etc", "passwd")
	if isSubPath(target, base) {
		t.Fatalf("path traversal via .. must be rejected: target=%q base=%q", target, base)
	}
}

func TestIsSubPath_SiblingDirectory(t *testing.T) {
	base := "/tmp/work/.light/reduction"
	target := "/tmp/work/.light/other"
	if isSubPath(target, base) {
		t.Fatalf("sibling directory must be rejected: target=%q base=%q", target, base)
	}
}

func TestIsSubPath_ExactSamePath(t *testing.T) {
	base := "/tmp/work/.light/reduction"
	// target == base 时 filepath.Rel 返回 ".",  rel != ".." 且不 startsWithDotDot
	// 严格来说写文件到根目录本身不应允许，但 isSubPath 只做子路径判断。
	// Write 方法会拼接 filepath.Dir(fullPath)，真实场景中 req.FilePath 总是子路径。
	if !isSubPath(base, base) {
		t.Fatalf("same path should be considered valid (rel='.'): base=%q", base)
	}
}

func TestStartsWithDotDot(t *testing.T) {
	cases := []struct {
		rel string
		ok  bool
	}{
		{"..", true},
		{"../foo", true},
		{"../bar/baz", true},
		{".", false},
		{"trunc/call_1", false},
		{"clear/call_2", false},
		{"foo", false},
		{"", false},
	}
	for _, c := range cases {
		got := startsWithDotDot(c.rel)
		if got != c.ok {
			t.Errorf("startsWithDotDot(%q) = %v, want %v", c.rel, got, c.ok)
		}
	}
}
