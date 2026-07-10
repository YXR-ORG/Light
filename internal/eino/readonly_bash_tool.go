package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// readOnlyBashPrefixes 是审查员 bash 允许的命令前缀（小写匹配）。
// 只允许查看和跑测试，禁止写/删/装包等副作用操作。
var readOnlyBashPrefixes = []string{
	"ls", "cat", "head", "tail", "wc", "file", "stat", "pwd", "echo",
	"find", "grep", "rg", "git status", "git diff", "git log", "git show", "git branch",
	"go test", "go vet", "go build", "go list", "go fmt", "gofmt",
	"npm test", "npm run test", "npx ", "yarn test", "pnpm test",
	"pytest", "python -m pytest", "python3 -m pytest",
	"cargo test", "cargo check", "make test", "make check",
	"diff", "tree", "which", "type",
}

// NewReadOnlyBashTool 创建只读 bash 工具，供独立审查员使用。
func NewReadOnlyBashTool(workDir string) *ReadOnlyBashTool {
	return &ReadOnlyBashTool{workDir: workDir}
}

// ReadOnlyBashTool 仅允许白名单内的只读/测试命令。
type ReadOnlyBashTool struct {
	workDir string
}

func (t *ReadOnlyBashTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "bash_exec",
		Desc: "在工作目录执行只读/测试类 shell 命令（ls、cat、git diff、go test、npm test 等）。禁止写文件、删文件、装包等有副作用的操作。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"cmd": {
				Type:     schema.String,
				Desc:     "要执行的命令（只读/测试类）",
				Required: true,
			},
		}),
	}, nil
}

func (t *ReadOnlyBashTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("bash_exec: invalid args: %w", err)
	}
	cmd := strings.TrimSpace(args.Cmd)
	if cmd == "" {
		return "命令为空", nil
	}
	if !isReadOnlyBashCmd(cmd) {
		return fmt.Sprintf("命令被拒绝：审查员只能执行只读/测试类命令，不允许：%s", cmd), nil
	}
	if isCritical(cmd) {
		return fmt.Sprintf("命令被拒绝：极危险命令不允许执行：%s", cmd), nil
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := exec.CommandContext(execCtx, "sh", "-c", cmd)
	c.Dir = t.workDir
	out, err := c.CombinedOutput()
	result := string(out)
	if len(result) > 50*1024 {
		result = result[:50*1024] + "\n\n[输出已截断，超过 50KB]"
	}
	if err != nil {
		if result == "" {
			return fmt.Sprintf("命令失败: %v", err), nil
		}
		return fmt.Sprintf("%s\n\n[exit error: %v]", result, err), nil
	}
	if result == "" {
		return "(无输出)", nil
	}
	return result, nil
}

func isReadOnlyBashCmd(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	// 管道/重定向写操作直接拒绝
	if strings.Contains(lower, " > ") || strings.Contains(lower, " >> ") ||
		strings.Contains(lower, " | tee ") || strings.Contains(lower, "rm ") ||
		strings.Contains(lower, "mv ") || strings.Contains(lower, "cp ") ||
		strings.Contains(lower, "chmod ") || strings.Contains(lower, "chown ") ||
		strings.Contains(lower, "npm install") || strings.Contains(lower, "pip install") ||
		strings.Contains(lower, "apt ") || strings.Contains(lower, "brew ") {
		return false
	}
	for _, p := range readOnlyBashPrefixes {
		if lower == p || strings.HasPrefix(lower, p+" ") || strings.HasPrefix(lower, p+"\t") {
			return true
		}
	}
	return false
}
