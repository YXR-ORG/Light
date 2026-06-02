package kb

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	// 检测需要重建 FTS5 的情况：
	// 1. 旧版 content= 外部表模式
	// 2. unicode61 tokenizer（对中文分词不可靠，换成 trigram）
	var ftsSQL string
	s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='chunks_fts'`).Scan(&ftsSQL)
	needRebuildFTS := false
	if ftsSQL != "" && (strings.Contains(ftsSQL, "content=") || strings.Contains(ftsSQL, "unicode61")) {
		slog.Info("migrate: dropping old FTS5 schema for rebuild", "reason", ftsSQL)
		s.db.Exec(`DROP TABLE IF EXISTS chunks_fts`)
		needRebuildFTS = true
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS documents (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  mime_type   TEXT NOT NULL,
  size        INTEGER DEFAULT 0,
  chunk_count INTEGER DEFAULT 0,
  status      TEXT DEFAULT 'pending',
  error       TEXT DEFAULT '',
  created_at  DATETIME
)`,
		`CREATE TABLE IF NOT EXISTS chunks (
  id          TEXT PRIMARY KEY,
  doc_id      TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  content     TEXT NOT NULL,
  chunk_index INTEGER NOT NULL,
  created_at  DATETIME
)`,
		// trigram tokenizer：把每个字符串切成所有3字符子串
		// 对中文完全可靠，任意子串（人名、地名等）都能被搜到
		// 代价：索引体积约为 unicode61 的 3-5 倍，但对本地知识库完全可接受
		`CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
  chunk_id UNINDEXED,
  doc_name UNINDEXED,
  chunk_index UNINDEXED,
  content,
  tokenize='trigram'
)`,
		`CREATE TABLE IF NOT EXISTS vectors (
  id       TEXT PRIMARY KEY,
  chunk_id TEXT NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
  embedding BLOB
)`,
		// 文档摘要表（TODO1）：上传文档后 LLM 异步生成，供两阶段检索使用
		`CREATE TABLE IF NOT EXISTS summaries (
  doc_id       TEXT PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
  doc_name     TEXT NOT NULL,
  summary      TEXT NOT NULL,
  key_entities TEXT DEFAULT '[]',  -- JSON 数组，如 ["张嘎","奶奶","鬼子"]
  created_at   DATETIME
)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w (sql: %.60s)", err, stmt)
		}
	}
	if needRebuildFTS {
		if err := s.rebuildFTSFromChunks(); err != nil {
			slog.Warn("migrate: rebuildFTS failed", "error", err)
		}
	}
	return nil
}

// rebuildFTSFromChunks 从 chunks 表重新填充 FTS5 索引（用于 schema 迁移后）
func (s *Store) rebuildFTSFromChunks() error {
	rows, err := s.db.Query(`
SELECT c.id, c.content, c.chunk_index, d.name
FROM chunks c JOIN documents d ON c.doc_id = d.id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for rows.Next() {
		var chunkID, content, docName string
		var chunkIndex int
		if err := rows.Scan(&chunkID, &content, &chunkIndex, &docName); err != nil {
			continue
		}
		tx.Exec(`INSERT INTO chunks_fts(chunk_id, doc_name, chunk_index, content) VALUES(?,?,?,?)`,
			chunkID, docName, chunkIndex, content)
	}
	count := 0
	s.db.QueryRow(`SELECT count(*) FROM chunks`).Scan(&count)
	slog.Info("rebuildFTS: done", "chunks", count)
	return tx.Commit()
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

// InsertChunks 批量写入分块并同步到 FTS5 索引
func (s *Store) InsertChunks(docID string, chunks []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 查询文档名（用于 FTS5 存储，方便检索结果展示）
	var docName string
	s.db.QueryRow(`SELECT name FROM documents WHERE id=?`, docID).Scan(&docName)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	for _, c := range chunks {
		chunkID := fmt.Sprintf("%s_%d", docID, c.ChunkIndex)
		if _, err := tx.Exec(
			`INSERT INTO chunks(id,doc_id,content,chunk_index,created_at) VALUES(?,?,?,?,?)`,
			chunkID, docID, c.Content, c.ChunkIndex, now,
		); err != nil {
			return err
		}
		// 同步写入 FTS5（独立存储，不依赖 content= 外部表）
		if _, err := tx.Exec(
			`INSERT INTO chunks_fts(chunk_id, doc_name, chunk_index, content) VALUES(?,?,?,?)`,
			chunkID, docName, c.ChunkIndex, c.Content,
		); err != nil {
			return fmt.Errorf("fts5 insert failed: %w", err)
		}
	}
	return tx.Commit()
}

// DeleteDocumentChunks 删除文档的所有分块及 FTS5 记录
func (s *Store) DeleteDocumentChunks(docID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 先查出所有 chunk_id，用于删除 FTS5
	rows, err := tx.Query(`SELECT id FROM chunks WHERE doc_id=?`, docID)
	if err != nil {
		return err
	}
	var chunkIDs []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		chunkIDs = append(chunkIDs, id)
	}
	rows.Close()

	for _, cid := range chunkIDs {
		tx.Exec(`DELETE FROM chunks_fts WHERE chunk_id=?`, cid)
	}
	if _, err := tx.Exec(`DELETE FROM chunks WHERE doc_id=?`, docID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM documents WHERE id=?`, docID); err != nil {
		return err
	}
	return tx.Commit()
}

// buildFTS5Query 把自然语言查询转为 trigram FTS5 查询。
// trigram 要求每个 token 至少 3 个 Unicode 字符。
// 返回：ftsTerms（>=3字符的词，用于FTS5），shortTerms（<3字符的词，用于LIKE）
func buildFTS5Query(query string) (andQuery, orQuery string, shortTerms []string) {
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return "", "", nil
	}

	var longParts []string
	for _, p := range parts {
		p = strings.ReplaceAll(p, `"`, `""`)
		if p == "" {
			continue
		}
		if len([]rune(p)) >= 3 {
			longParts = append(longParts, `"`+p+`"`)
		} else {
			shortTerms = append(shortTerms, p)
		}
	}

	if len(longParts) > 0 {
		andQuery = strings.Join(longParts, " ")
		orQuery = strings.Join(longParts, " OR ")
	}
	return andQuery, orQuery, shortTerms
}

// searchByLike 对短词（<3字符，trigram不支持）用 LIKE 查询补充
func (s *Store) searchByLike(terms []string, topK int) ([]SearchResult, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	// 构建 WHERE content LIKE '%x%' AND content LIKE '%y%'
	conditions := make([]string, len(terms))
	args := make([]interface{}, len(terms)+1)
	for i, t := range terms {
		conditions[i] = "content LIKE ?"
		args[i] = "%" + t + "%"
	}
	args[len(terms)] = topK
	query := fmt.Sprintf(`
SELECT c.content, c.chunk_index, d.name
FROM chunks c JOIN documents d ON c.doc_id = d.id
WHERE %s
LIMIT ?`, strings.Join(conditions, " AND "))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
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

// Search 四路融合检索 + 摘要层两阶段过滤
//
// 阶段一（摘要层）：若 summaries 表有数据，先用 query 匹配摘要/实体，
//   得到相关文档 ID 集合，后续检索优先在该范围内进行。
//   若摘要层无命中（如文档摘要尚未生成），不过滤，走全量检索。
//
// 阶段二（四路融合）：
//   路径1: FTS5 AND（精度高）
//   路径2: 向量余弦相似度（语义）
//   路径3: FTS5 OR（召回补充）
//   路径4: LIKE（短词兜底）
func (s *Store) Search(query string, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	if topK > 20 {
		topK = 20
	}

	// --- 阶段一：摘要层定位相关文档 ---
	var docFilter map[string]bool // nil = 不过滤
	summaries, err := s.SearchSummaries(query)
	if err == nil && len(summaries) > 0 {
		docFilter = make(map[string]bool, len(summaries))
		for _, ds := range summaries {
			docFilter[ds.DocName] = true
		}
		slog.Debug("kbstore: summary filter", "query", query, "docs", len(docFilter))
	}

	seen := make(map[string]bool)
	var primary []SearchResult   // 命中摘要过滤的结果（排前面）
	var fallback []SearchResult  // 未命中摘要过滤的结果（兜底）

	addUniq := func(r SearchResult) {
		if seen[r.Content] {
			return
		}
		seen[r.Content] = true
		if docFilter == nil || docFilter[r.DocName] {
			primary = append(primary, r)
		} else {
			fallback = append(fallback, r)
		}
	}

	// --- 路径1：FTS5 AND 查询（精度高）---
	andQuery, orQuery, shortTerms := buildFTS5Query(query)
	if andQuery != "" {
		r, _ := s.doSearch(andQuery, topK)
		for _, x := range r {
			addUniq(x)
		}
	}

	// --- 路径2：向量检索（语义）---
	vecResults, vecErr := s.VectorSearch(query, topK)
	if vecErr != nil {
		slog.Debug("vector search unavailable", "error", vecErr)
	} else {
		for _, vr := range vecResults {
			addUniq(vr.SearchResult)
		}
	}

	// --- 路径3：FTS5 OR 补充（召回不足时）---
	if len(primary)+len(fallback) < topK && orQuery != "" && orQuery != andQuery {
		r, _ := s.doSearch(orQuery, topK)
		for _, x := range r {
			addUniq(x)
		}
	}

	// --- 路径4：短词 LIKE 补充 ---
	if len(shortTerms) > 0 {
		r, _ := s.searchByLike(shortTerms, topK)
		for _, x := range r {
			addUniq(x)
		}
	}

	// 合并：命中摘要的结果优先，兜底结果补充
	allResults := append(primary, fallback...)
	if len(allResults) > topK {
		allResults = allResults[:topK]
	}

	slog.Info("kbstore: search", "query", query, "results", len(allResults),
		"summary_filter", len(docFilter), "vec_available", vecErr == nil)
	return allResults, nil
}

func (s *Store) doSearch(ftsQuery string, topK int) ([]SearchResult, error) {
	rows, err := s.db.Query(`
SELECT content, chunk_index, doc_name
FROM chunks_fts
WHERE chunks_fts MATCH ?
ORDER BY rank
LIMIT ?`, ftsQuery, topK)
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

// InsertVectors 批量写入向量
func (s *Store) InsertVectors(docID string, chunks []Chunk, vecs [][]float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, vec := range vecs {
		chunkID := fmt.Sprintf("%s_%d", docID, chunks[i].ChunkIndex)
		vecID := fmt.Sprintf("v_%s", chunkID)
		blob := Float32SliceToBytes(vec)
		if _, err := tx.Exec(
			`INSERT OR REPLACE INTO vectors(id, chunk_id, embedding) VALUES(?,?,?)`,
			vecID, chunkID, blob,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertSummary 插入或更新文档摘要（LLM 生成后调用）
func (s *Store) UpsertSummary(docID, docName, summary, keyEntities string) error {
	_, err := s.db.Exec(`
INSERT INTO summaries(doc_id, doc_name, summary, key_entities, created_at)
VALUES(?,?,?,?,?)
ON CONFLICT(doc_id) DO UPDATE SET
  summary=excluded.summary,
  key_entities=excluded.key_entities,
  created_at=excluded.created_at`,
		docID, docName, summary, keyEntities, time.Now().Format(time.RFC3339))
	return err
}

// DocSummary 文档摘要 DTO
type DocSummary struct {
	DocID       string
	DocName     string
	Summary     string
	KeyEntities string // JSON 数组
}

// SearchSummaries 在摘要层搜索，返回相关文档名列表（两阶段检索第一阶段）
func (s *Store) SearchSummaries(query string) ([]DocSummary, error) {
	rows, err := s.db.Query(`
SELECT doc_id, doc_name, summary, key_entities
FROM summaries
WHERE summary LIKE ? OR key_entities LIKE ?`,
		"%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []DocSummary
	for rows.Next() {
		var d DocSummary
		rows.Scan(&d.DocID, &d.DocName, &d.Summary, &d.KeyEntities)
		results = append(results, d)
	}
	return results, rows.Err()
}

// GetAllSummaries 返回所有文档摘要（用于构建上下文）
func (s *Store) GetAllSummaries() ([]DocSummary, error) {
	rows, err := s.db.Query(`SELECT doc_id, doc_name, summary, key_entities FROM summaries ORDER BY doc_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []DocSummary
	for rows.Next() {
		var d DocSummary
		rows.Scan(&d.DocID, &d.DocName, &d.Summary, &d.KeyEntities)
		results = append(results, d)
	}
	return results, rows.Err()
}

// AllChunksForDoc 返回某文档的所有 chunks（重建索引用）
func (s *Store) AllChunksForDoc(docID string) ([]Chunk, error) {
	rows, err := s.db.Query(`SELECT content, chunk_index FROM chunks WHERE doc_id=? ORDER BY chunk_index`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		rows.Scan(&c.Content, &c.ChunkIndex)
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// AllReadyDocuments 返回所有 status=ready 的文档（重建索引用）
func (s *Store) AllReadyDocuments() ([]KBDocument, error) {
	rows, err := s.db.Query(`SELECT id, name FROM documents WHERE status='ready'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []KBDocument
	for rows.Next() {
		var d KBDocument
		rows.Scan(&d.ID, &d.Name)
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// DeleteVectorsForDoc 删除某文档的所有向量
func (s *Store) DeleteVectorsForDoc(docID string) error {
	_, err := s.db.Exec(`DELETE FROM vectors WHERE chunk_id LIKE ?`, docID+"_%")
	return err
}

// DeleteSummaryForDoc 删除某文档的摘要
func (s *Store) DeleteSummaryForDoc(docID string) error {
	_, err := s.db.Exec(`DELETE FROM summaries WHERE doc_id=?`, docID)
	return err
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
