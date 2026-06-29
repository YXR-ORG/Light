package kb

import (
	"strings"
	"sync"

	"github.com/go-ego/gse"
)

// 中文分词器（gse），用于检索前的 query 预处理。
//
// 设计取舍：只在 query 侧分词，不碰文档索引。
// 文档侧已有 trigram FTS5（任意子串可搜，可靠），无需重建索引；
// query 侧分词是为了让停用词清洗、同义词归一化能按"词"操作，
// 并让 buildFTS5Query 的 strings.Fields 真正切出 token（中文无空格）。
//
// gse 默认嵌入中文词典（~4MB），首次加载约 50-100ms，之后无开销。

var (
	segOnce    sync.Once
	segmenter  *gse.Segmenter
	segmentErr error
)

// getSegmenter 懒加载分词器（全局单例）。
// 失败时返回 error，调用方需降级为按空格切分。
func getSegmenter() (*gse.Segmenter, error) {
	segOnce.Do(func() {
		seg, err := gse.NewEmbed("zh")
		if err != nil {
			segmentErr = err
			return
		}
		segmenter = &seg
	})
	return segmenter, segmentErr
}

// Tokenize 中文分词，返回有意义的词列表。
// 过滤标点、空白、单字噪声（单字对检索价值低且易误匹配）。
// 分词不可用时降级为按空格切分，保持原有行为。
func Tokenize(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	seg, err := getSegmenter()
	if err != nil || seg == nil {
		// 降级：按空格切（英文/已带空格的 query 仍可用）
		return strings.Fields(text)
	}

	// CutSearch 搜索引擎模式，切得更细，利于召回
	tokens := seg.CutSearch(text, true)

	result := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		// 过滤单字：中文单字歧义大、英文单字无语义，检索价值低
		if len([]rune(t)) <= 1 {
			continue
		}
		// 过滤纯标点/符号
		if isPunctOrSymbol(t) {
			continue
		}
		result = append(result, t)
	}
	return result
}

// CutToQuery 分词后用空格拼接，供 FTS5 的 buildFTS5Query 使用。
// 带 token 之间空格后，strings.Fields 才能正确切分中文 query。
func CutToQuery(text string) string {
	tokens := Tokenize(text)
	return strings.Join(tokens, " ")
}

// isPunctOrSymbol 判断 token 是否为纯标点/符号（无检索价值）。
func isPunctOrSymbol(s string) bool {
	for _, r := range s {
		if !isPunctRune(r) {
			return false
		}
	}
	return true
}

// isPunctRune 粗略判断是否为标点/符号 rune。
func isPunctRune(r rune) bool {
	switch {
	case r >= 0x20 && r <= 0x2F: // 空格 ! " # $ % & ' ( ) * + , - . /
		return true
	case r >= 0x3A && r <= 0x40: // : ; < = > ? @
		return true
	case r >= 0x5B && r <= 0x60: // [ \ ] ^ _ `
		return true
	case r >= 0x7B && r <= 0x7E: // { | } ~
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK 标点 ，。！？等
		return true
	case r >= 0xFF00 && r <= 0xFFEF: // 全角符号
		return true
	}
	return false
}
