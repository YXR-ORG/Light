// Package kb 提供文档解析、分块和知识库存储操作。
package kb

import (
	"archive/zip"
	"bytes"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/dslipak/pdf"
	"github.com/nguyenthenguyen/docx"
	"github.com/xuri/excelize/v2"
)

// ParseText 从文件数据中提取纯文本，根据扩展名选择解析器。
// 解析失败时返回错误，调用方决定是否跳过。
func ParseText(data []byte, filename string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return parsePDF(data)
	case ".docx":
		return parseDocx(data)
	case ".xlsx", ".xls":
		return parseExcel(data, filename)
	default:
		// 纯文本类：txt md csv json yaml xml html go py js ts java sql sh rs cpp c
		return string(data), nil
	}
}

// parsePDF 用 dslipak/pdf 提取 PDF 文本层。扫描版 PDF 无文本层时返回空字符串。
func parsePDF(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdf open failed: %w", err)
	}

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			slog.Warn("parsePDF: get page text failed", "page", i, "error", err)
			continue
		}
		sb.WriteString(text)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// parseDocx 用 nguyenthenguyen/docx 提取 Word 段落文本。
func parseDocx(data []byte) (string, error) {
	r, err := docx.ReadDocxFromMemory(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docx read failed: %w", err)
	}
	defer r.Close()
	doc := r.Editable()
	return doc.GetContent(), nil
}

// parseExcel 用 excelize 逐 sheet 逐行转为 TSV 文本。
func parseExcel(data []byte, _ string) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("excel open failed: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			slog.Warn("parseExcel: get rows failed", "sheet", sheet, "error", err)
			continue
		}
		sb.WriteString("## Sheet: ")
		sb.WriteString(sheet)
		sb.WriteByte('\n')
		for _, row := range rows {
			sb.WriteString(strings.Join(row, "\t"))
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// isZipBased 检查文件是否为 zip 格式（docx/xlsx 都是 zip）
func isZipBased(data []byte) bool {
	_, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	return err == nil
}
