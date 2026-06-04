package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const fileReadLimit = 100 * 1024 // 100KB

// fileTool 提供受限于 workDir 的文件操作基础。
type fileTool struct {
	workDir string
}

// safePath 验证 path 在 workDir 内，返回绝对路径。
// 若 path 是相对路径，以 workDir 为根。
func (f *fileTool) safePath(rel string) (string, error) {
	abs := rel
	if !filepath.IsAbs(rel) {
		abs = filepath.Join(f.workDir, rel)
	}
	abs = filepath.Clean(abs)
	root := filepath.Clean(f.workDir)
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径越界：%q 不在工作目录内", rel)
	}
	return abs, nil
}

// NewFileTools 返回四个文件操作工具的列表，均限制在 workDir 内。
func NewFileTools(workDir string) []tool.BaseTool {
	base := &fileTool{workDir: workDir}
	return []tool.BaseTool{
		&ReadFileTool{base},
		&WriteFileTool{base},
		&ListDirTool{base},
		&MakeDirTool{base},
	}
}

// ---- read_file ----

// ReadFileTool 读取工作目录内文件内容。
type ReadFileTool struct{ *fileTool }

func (t *ReadFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_file",
		Desc: "读取工作目录内的文件内容（最多 100KB）",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "相对于工作目录的文件路径", Required: true},
		}),
	}, nil
}

func (t *ReadFileTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("read_file: invalid args: %w", err)
	}
	abs, err := t.safePath(args.Path)
	if err != nil {
		return err.Error(), nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Sprintf("读取失败: %v", err), nil
	}
	body := string(data)
	if len(data) > fileReadLimit {
		body = string(data[:fileReadLimit]) + "\n\n[内容已截断，超过 100KB]"
	}
	// 追加产物标记（供前端在“本次涉及的文件”区展示，可点击打开）
	return EmbedArtifact(body, Artifact{
		Type:    "file",
		Action:  "read",
		Title:   filepath.Base(args.Path),
		Path:    args.Path,
		AbsPath: abs,
		Bytes:   len(data),
	}), nil
}

// ---- write_file ----

// WriteFileTool 将内容写入工作目录内文件。
type WriteFileTool struct{ *fileTool }

func (t *WriteFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "write_file",
		Desc: "将内容写入工作目录内的文件（自动创建父目录）",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":    {Type: schema.String, Desc: "相对于工作目录的文件路径", Required: true},
			"content": {Type: schema.String, Desc: "要写入的文件内容", Required: true},
		}),
	}, nil
}

func (t *WriteFileTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("write_file: invalid args: %w", err)
	}
	abs, err := t.safePath(args.Path)
	if err != nil {
		return err.Error(), nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return fmt.Sprintf("创建父目录失败: %v", err), nil
	}
	if err := os.WriteFile(abs, []byte(args.Content), 0644); err != nil {
		return fmt.Sprintf("写入失败: %v", err), nil
	}
	// 返回人类可读结果给 LLM，同时通过产物标记把结构化数据传给前端渲染
	humanText := fmt.Sprintf("文件已写入: %s（%d 字节）", args.Path, len(args.Content))
	return EmbedArtifact(humanText, Artifact{
		Type:    "file",
		Action:  "write",
		Title:   filepath.Base(args.Path),
		Path:    args.Path,
		AbsPath: abs,
		Bytes:   len(args.Content),
	}), nil
}

// ---- list_dir ----

// ListDirTool 列出工作目录内目录内容。
type ListDirTool struct{ *fileTool }

func (t *ListDirTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_dir",
		Desc: "列出工作目录内的目录内容（文件名、大小、修改时间）",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "相对于工作目录的目录路径，默认为根目录（可为空）", Required: false},
		}),
	}, nil
}

func (t *ListDirTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &args)
	if args.Path == "" {
		args.Path = "."
	}
	abs, err := t.safePath(args.Path)
	if err != nil {
		return err.Error(), nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return fmt.Sprintf("读取目录失败: %v", err), nil
	}
	if len(entries) == 0 {
		return "（空目录）", nil
	}
	var sb strings.Builder
	for _, e := range entries {
		info, _ := e.Info()
		if e.IsDir() {
			sb.WriteString(fmt.Sprintf("[目录] %s/\n", e.Name()))
		} else {
			size := int64(0)
			mtime := time.Time{}
			if info != nil {
				size = info.Size()
				mtime = info.ModTime()
			}
			sb.WriteString(fmt.Sprintf("[文件] %-40s %8d B  %s\n",
				e.Name(), size, mtime.Format("2006-01-02 15:04")))
		}
	}
	return sb.String(), nil
}

// ---- make_dir ----

// MakeDirTool 在工作目录内创建目录。
type MakeDirTool struct{ *fileTool }

func (t *MakeDirTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "make_dir",
		Desc: "在工作目录内创建目录（含所有父目录）",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "相对于工作目录的目录路径", Required: true},
		}),
	}, nil
}

func (t *MakeDirTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("make_dir: invalid args: %w", err)
	}
	abs, err := t.safePath(args.Path)
	if err != nil {
		return err.Error(), nil
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return fmt.Sprintf("创建目录失败: %v", err), nil
	}
	return fmt.Sprintf("已创建目录: %s", args.Path), nil
}
