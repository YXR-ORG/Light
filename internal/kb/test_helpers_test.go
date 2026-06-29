package kb

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newTestStore 为测试创建一个内存级或临时文件的 Store。
// 使用临时目录（而非 :memory:），因为 FTS5 虚拟表在真实文件上行为更可靠。
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := openStore(dir)
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	return s
}

// 兜底：避免 sql 包未使用告警（openStore 内部已用）
var _ = sql.Open
