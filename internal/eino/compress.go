package eino

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/model"
)

// fileBackend 是 reduction 中间件使用的文件存储后端。
// 卸载的工具输出写入 workDir/.light/reduction/ 目录，
// agent 可通过已有的 read_file 工具回读。
type fileBackend struct {
	rootDir string
}

func (b *fileBackend) Write(_ context.Context, req *filesystem.WriteRequest) error {
	if req == nil || req.FilePath == "" {
		return fmt.Errorf("reduction backend: empty write request")
	}
	// req.FilePath 由 reduction 中间件生成，格式为 ${rootDir}/trunc/{tool_call_id} 或 ${rootDir}/clear/{tool_call_id}
	// 确保目标路径在 rootDir 下（防止路径穿越）
	fullPath := req.FilePath
	if !isSubPath(fullPath, b.rootDir) {
		fullPath = filepath.Join(b.rootDir, filepath.Base(req.FilePath))
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("reduction backend: mkdir failed: %w", err)
	}
	if err := os.WriteFile(fullPath, []byte(req.Content), 0o644); err != nil {
		return fmt.Errorf("reduction backend: write failed: %w", err)
	}
	return nil
}

// isSubPath 检查 target 是否在 base 目录下。
func isSubPath(target, base string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !startsWithDotDot(rel)
}

func startsWithDotDot(rel string) bool {
	return len(rel) >= 2 && rel[0] == '.' && rel[1] == '.'
}

// BuildCompressionMiddlewares 构造上下文压缩中间件链（reduction + summarization）。
// 返回的中间件按顺序挂载到 adk.ChatModelAgentConfig.Handlers。
//
// 参数：
//   - llm：用于生成摘要的模型（通常与主模型相同，可后续优化为便宜模型）
//   - workDir：任务工作目录，卸载文件存入 workDir/.light/reduction/
//
// 压缩策略（两层防线）：
//  1. reduction（先触发）：机械裁剪大工具输出，零 LLM 成本
//  2. summarization（后触发）：token 仍超标时，LLM 语义压缩
//
// 如果 workDir 不可写或构造失败，返回 nil 中间件（降级为不压缩）。
func BuildCompressionMiddlewares(ctx context.Context, llm model.ToolCallingChatModel, workDir string) []adk.ChatModelAgentMiddleware {
	var handlers []adk.ChatModelAgentMiddleware

	// --- reduction 中间件 ---
	reductionDir := filepath.Join(workDir, ".light", "reduction")
	if err := os.MkdirAll(reductionDir, 0o755); err != nil {
		// workDir 不可写，跳过 reduction（降级）
		return nil
	}

	reductionMW, err := reduction.New(ctx, &reduction.Config{
		Backend:            &fileBackend{rootDir: reductionDir},
		ReadFileToolName:   "read_file", // 我们已有 read_file 工具
		RootDir:            reductionDir,
		MaxLengthForTrunc:  50000, // 单次工具输出 >50KB 才截断
		MaxTokensForClear:  120000,
		ClearRetentionSuffixLimit: 3, // 保留最近 3 条消息不清理
	})
	if err != nil {
		// 构造失败，跳过 reduction
		return nil
	}
	handlers = append(handlers, reductionMW)

	// --- summarization 中间件 ---
	if llm == nil {
		// 没有 LLM 无法做 summarization，只返回 reduction
		return handlers
	}

	transcriptPath := filepath.Join(workDir, ".light", "transcript.md")
	summarizationMW, err := summarization.New(ctx, &summarization.Config{
		Model: llm,
		Trigger: &summarization.TriggerCondition{
			ContextTokens: 120000, // 与 reduction 的 MaxTokensForClear 一致
		},
		TranscriptFilePath: transcriptPath, // 完整对话原文落盘，摘要中提示 agent 回读
	})
	if err != nil {
		// summarization 构造失败，只返回 reduction
		return handlers
	}
	handlers = append(handlers, summarizationMW)

	return handlers
}
