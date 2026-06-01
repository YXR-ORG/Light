package kb

import "strings"

const (
	chunkSize    = 400 // 字符数（保守估算 ~512 tokens）
	chunkOverlap = 64  // 重叠字符数
	minChunkSize = 50  // 小于此值合并到前一块
)

// Chunk 表示一个文本分块
type Chunk struct {
	Content    string
	ChunkIndex int
}

// SplitChunks 将文本按段落优先策略分块。
// 优先在 \n\n 边界分割，避免在句子中间截断。
func SplitChunks(text string) []Chunk {
	if len(text) == 0 {
		return nil
	}

	// 按段落分割
	paragraphs := strings.Split(text, "\n\n")
	var chunks []Chunk
	var buf strings.Builder
	idx := 0

	flush := func() {
		content := strings.TrimSpace(buf.String())
		if len(content) == 0 {
			return
		}
		if len(chunks) > 0 && len(content) < minChunkSize {
			// 合并到前一块
			chunks[len(chunks)-1].Content += "\n" + content
		} else {
			chunks = append(chunks, Chunk{Content: content, ChunkIndex: idx})
			idx++
		}
		buf.Reset()
	}

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// 段落本身超过 chunkSize，按字符强制分割
		if len(para) > chunkSize {
			// 先 flush 当前 buf
			if buf.Len() > 0 {
				flush()
			}
			for len(para) > 0 {
				end := chunkSize
				if end > len(para) {
					end = len(para)
				}
				piece := para[:end]
				para = para[end:]
				// 加重叠
				if len(para) > 0 && chunkOverlap > 0 {
					overlap := chunkOverlap
					if overlap > len(piece) {
						overlap = len(piece)
					}
					para = piece[len(piece)-overlap:] + para
				}
				chunks = append(chunks, Chunk{Content: piece, ChunkIndex: idx})
				idx++
			}
			continue
		}

		// 加入 buf，超过 chunkSize 时 flush
		if buf.Len()+len(para)+2 > chunkSize && buf.Len() > 0 {
			flush()
			// 加重叠：把上一块末尾 chunkOverlap 字符带入新块
			if len(chunks) > 0 {
				prev := chunks[len(chunks)-1].Content
				if len(prev) > chunkOverlap {
					buf.WriteString(prev[len(prev)-chunkOverlap:])
					buf.WriteString("\n\n")
				}
			}
		}
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(para)
	}
	flush()

	return chunks
}
