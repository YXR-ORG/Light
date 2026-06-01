package kb

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// KBDocument 文档元数据 DTO（与 handler 层共用）
type KBDocument struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	MimeType   string `json:"mime_type"`
	Size       int64  `json:"size"`
	ChunkCount int    `json:"chunk_count"`
	Status     string `json:"status"` // pending|processing|ready|error
	Error      string `json:"error"`
	CreatedAt  string `json:"created_at"`
}

// SearchResult 单条检索结果
type SearchResult struct {
	DocName    string `json:"doc_name"`
	ChunkIndex int    `json:"chunk_index"`
	Content    string `json:"content"`
}

// Store 管理单个知识库的 kb.db
type Store struct {
	db  *sql.DB
	dir string // kb 根目录，含 files/ 子目录
	mu  sync.Mutex
}

var (
	storesMu sync.Mutex
	stores   = map[string]*Store{} // kbID -> Store
)

// GetStore 获取或创建知识库 Store（线程安全）
func GetStore(kbID, kbDir string) (*Store, error) {
	storesMu.Lock()
	defer storesMu.Unlock()

	if s, ok := stores[kbID]; ok {
		return s, nil
	}
	s, err := openStore(kbDir)
	if err != nil {
		return nil, err
	}
	stores[kbID] = s
	return s, nil
}

// CloseStore 关闭并从缓存移除（删除知识库时调用）
func CloseStore(kbID string) {
	storesMu.Lock()
	defer storesMu.Unlock()
	if s, ok := stores[kbID]; ok {
		s.db.Close()
		delete(stores, kbID)
	}
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS documents (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  mime_type   TEXT NOT NULL,
  size        INTEGER DEFAULT 0,
  chunk_count INTEGER DEFAULT 0,
  status      TEXT DEFAULT 'pending',
  error       TEXT DEFAULT '',
  created_at  DATETIME
);

CREATE TABLE IF NOT EXISTS chunks (
  id          TEXT PRIMARY KEY,
  doc_id      TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  content     TEXT NOT NULL,
  chunk_index INTEGER NOT NULL,
  created_at  DATETIME
);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
  content,
  content='chunks',
  content_rowid='rowid',
  tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS vectors (
  id       TEXT PRIMARY KEY,
  chunk_id TEXT NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
  embedding BLOB
);
`)
	return err
}

// newID 生成简单 UUID（复用 storage 包逻辑，避免循环依赖）
func newID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// InsertDocument 插入文档记录（status=pending）
func (s *Store) InsertDocument(name, mimeType string, size int64) (string, error) {
	id := newID()
	_, err := s.db.Exec(
		`INSERT INTO documents(id,name,mime_type,size,status,created_at) VALUES(?,?,?,?,'pending',?)`,
		id, name, mimeType, size, time.Now().Format(time.RFC3339),
	)
	return id, err
}

// UpdateDocumentStatus 更新文档状态
func (s *Store) UpdateDocumentStatus(docID, status, errMsg string, chunkCount int) error {
	_, err := s.db.Exec(
		`UPDATE documents SET status=?, error=?, chunk_count=? WHERE id=?`,
		status, errMsg, chunkCount, docID,
	)
	return err
}

// InsertChunks 批量写入分块并重建 FTS5 索引
func (s *Store) InsertChunks(docID string, chunks []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	for _, c := range chunks {
		id := fmt.Sprintf("%s_%d", docID, c.ChunkIndex)
		if _, err := tx.Exec(
			`INSERT INTO chunks(id,doc_id,content,chunk_index,created_at) VALUES(?,?,?,?,?)`,
			id, docID, c.Content, c.ChunkIndex, now,
		); err != nil {
			return err
		}
	}
	// 重建 FTS5 索引
	if _, err := tx.Exec(`INSERT INTO chunks_fts(chunks_fts) VALUES('rebuild')`); err != nil {
		slog.Warn("kbstore: fts5 rebuild failed", "error", err)
	}
	return tx.Commit()
}

// DeleteDocumentChunks 删除文档的所有分块（级联删除 FTS5 由触发器处理，这里手动重建）
func (s *Store) DeleteDocumentChunks(docID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM chunks WHERE doc_id=?`, docID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM documents WHERE id=?`, docID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO chunks_fts(chunks_fts) VALUES('rebuild')`); err != nil {
		slog.Warn("kbstore: fts5 rebuild failed after delete", "error", err)
	}
	return tx.Commit()
}

// Search 用 FTS5 检索，返回最多 topK 条结果
func (s *Store) Search(query string, topK int) ([]SearchResult, error) {
	if topK <= 0 || topK > 10 {
		topK = 5
	}
	rows, err := s.db.Query(`
SELECT c.content, c.chunk_index, d.name
FROM chunks_fts
JOIN chunks c ON chunks_fts.rowid = c.rowid
JOIN documents d ON c.doc_id = d.id
WHERE chunks_fts MATCH ?
ORDER BY rank
LIMIT ?`, query, topK)
	if err != nil {
		return nil, fmt.Errorf("fts5 search failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.Content, &r.ChunkIndex, &r.DocName); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ListDocuments 列出知识库内所有文档
func (s *Store) ListDocuments() ([]KBDocument, error) {
	rows, err := s.db.Query(
		`SELECT id,name,mime_type,size,chunk_count,status,error,created_at FROM documents ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []KBDocument
	for rows.Next() {
		var d KBDocument
		if err := rows.Scan(&d.ID, &d.Name, &d.MimeType, &d.Size, &d.ChunkCount, &d.Status, &d.Error, &d.CreatedAt); err != nil {
			continue
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// GetDocumentStatus 查询单个文档状态
func (s *Store) GetDocumentStatus(docID string) (string, error) {
	var status string
	err := s.db.QueryRow(`SELECT status FROM documents WHERE id=?`, docID).Scan(&status)
	return status, err
}

// FilesDir 返回原始文件存储目录
func (s *Store) FilesDir() string {
	return filepath.Join(s.dir, "files")
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0755); err != nil {
		return nil, fmt.Errorf("kbstore: mkdir failed: %w", err)
	}
	dbPath := filepath.Join(dir, "kb.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("kbstore: open db failed: %w", err)
	}
	s := &Store{db: db, dir: dir}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
