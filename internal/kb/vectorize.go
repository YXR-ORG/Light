package kb

import (
	"fmt"
	"log/slog"
	"sort"
)

// VectorizeDocument 对文档所有 chunk 生成向量并写入 vectors 表（异步调用）
func VectorizeDocument(store *Store, docID string, chunks []Chunk) error {
	p, err := getEmbedder()
	if err != nil {
		return fmt.Errorf("embedder not available: %w", err)
	}
	_ = p // getEmbedder 内部已初始化全局 pipeline

	// 批量向量化，每批 32 条（hugot 纯 Go backend 推荐）
	const batchSize = 32
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]
		texts := make([]string, len(batch))
		for j, c := range batch {
			texts[j] = c.Content
		}
		vecs, err := Embed(texts)
		if err != nil {
			return fmt.Errorf("embed batch [%d:%d]: %w", i, end, err)
		}
		if err := store.InsertVectors(docID, batch, vecs); err != nil {
			return fmt.Errorf("insert vectors [%d:%d]: %w", i, end, err)
		}
	}
	slog.Info("vectorize: done", "doc_id", docID, "chunks", len(chunks))
	return nil
}

// VectorSearchResult 向量检索结果
type VectorSearchResult struct {
	SearchResult
	Score float32
}

// VectorSearch 向量相似度检索，返回最相似的 topK 条
func (s *Store) VectorSearch(query string, topK int) ([]VectorSearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	// 查询向量
	vecs, err := Embed([]string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	queryVec := vecs[0]

	// 读取所有向量（全量扫描，适合本地小规模知识库）
	rows, err := s.db.Query(`
SELECT v.id, v.chunk_id, v.embedding, c.content, c.chunk_index, d.name
FROM vectors v
JOIN chunks c ON v.chunk_id = c.id
JOIN documents d ON c.doc_id = d.id`)
	if err != nil {
		return nil, fmt.Errorf("query vectors: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		VectorSearchResult
	}
	var candidates []candidate
	for rows.Next() {
		var id, chunkID string
		var embBlob []byte
		var r SearchResult
		if err := rows.Scan(&id, &chunkID, &embBlob, &r.Content, &r.ChunkIndex, &r.DocName); err != nil {
			continue
		}
		vec := BytesToFloat32Slice(embBlob)
		if len(vec) == 0 {
			continue
		}
		score := CosineSim(queryVec, vec)
		candidates = append(candidates, candidate{VectorSearchResult{r, score}})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 按相似度降序排列
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	results := make([]VectorSearchResult, len(candidates))
	for i, c := range candidates {
		results[i] = c.VectorSearchResult
	}
	return results, nil
}
