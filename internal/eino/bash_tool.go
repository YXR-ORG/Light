package eino

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

// defaultBashBlacklist 是内置危险命令规则，每行一条 glob 模式（大小写不敏感）。
const defaultBashBlacklist = `rm -rf /
rm -rf ~
sudo
mkfs
dd if=
curl * | sh
curl * | bash
wget * | sh
wget * | bash
> /dev/
chmod 777 /
:(){ :|:& };:`

// BashConfirmRequest 由 BashTool 发出，等待前端确认。
type BashConfirmRequest struct {
	ID  string // 唯一标识，前端用 ConfirmBash(id, approved) 回复
	Cmd string // 待执行命令
}

// BashStepEmitter 由 TaskAgent/TaskHandler 注入，用于推送 task:step 事件。
type BashStepEmitter func(stepType, content, cmd, confirmID string)

// BashTool 实现 eino InvokableTool，在工作目录执行 shell 命令。
// 黑名单命令先向前端发送 bash_confirm 事件，阻塞等待用户确认后才执行。
type BashTool struct {
	workDir     string
	emitter     BashStepEmitter
	blacklist   []string // glob 模式列表
	confirmsMu  sync.Mutex
	confirms    map[string]chan bool // confirmID → answer channel
}

// NewBashTool 创建 BashTool。
// blacklistRules: 换行分隔的 glob 模式，若为空则使用默认内置规则。
// emitter: 推送 task:step 事件的回调。
func NewBashTool(workDir, blacklistRules string, emitter BashStepEmitter) *BashTool {
	if blacklistRules == "" {
		blacklistRules = defaultBashBlacklist
	}
	rules := parseBlacklist(blacklistRules)
	return &BashTool{
		workDir:   workDir,
		emitter:   emitter,
		blacklist:  rules,
		confirms:   make(map[string]chan bool),
	}
}

func parseBlacklist(raw string) []string {
	var rules []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			rules = append(rules, strings.ToLower(line))
		}
	}
	return rules
}

// isDangerous 检查命令是否命中黑名单（glob 通配，大小写不敏感）。
func (t *BashTool) isDangerous(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	for _, rule := range t.blacklist {
		// 先尝试 glob 匹配
		matched, err := filepath.Match(rule, lower)
		if err == nil && matched {
			return true
		}
		// 包含匹配（glob 不含子串语义时作为 fallback）
		if strings.Contains(lower, rule) {
			return true
		}
	}
	return false
}

// Confirm 由 TaskHandler.ConfirmBash 调用，将用户的确认/拒绝结果写入等待 channel。
func (t *BashTool) Confirm(confirmID string, approved bool) {
	t.confirmsMu.Lock()
	ch, ok := t.confirms[confirmID]
	t.confirmsMu.Unlock()
	if ok {
		select {
		case ch <- approved:
		default:
		}
	}
}

func (t *BashTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "bash_exec",
		Desc: "在工作目录中执行 shell 命令。危险命令（如 rm -rf、sudo 等）会先请求用户确认。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"cmd": {
				Type:     schema.String,
				Desc:     "要执行的 shell 命令",
				Required: true,
			},
			"timeout_sec": {
				Type:     schema.Integer,
				Desc:     "执行超时秒数，默认 30，最大 300",
				Required: false,
			},
		}),
	}, nil
}

func (t *BashTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Cmd        string `json:"cmd"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("bash_exec: invalid args: %w", err)
	}
	if args.Cmd == "" {
		return "错误：cmd 不能为空", nil
	}
	timeout := time.Duration(args.TimeoutSec) * time.Second
	if timeout <= 0 || timeout > 300*time.Second {
		timeout = 30 * time.Second
	}

	// 黑名单检查 → 需要用户确认
	if t.isDangerous(args.Cmd) {
		confirmID := uuid.New().String()
		ch := make(chan bool, 1)

		t.confirmsMu.Lock()
		t.confirms[confirmID] = ch
		t.confirmsMu.Unlock()

		defer func() {
			t.confirmsMu.Lock()
			delete(t.confirms, confirmID)
			t.confirmsMu.Unlock()
		}()

		// 通知前端弹窗
		if t.emitter != nil {
			t.emitter("bash_confirm", "", args.Cmd, confirmID)
		}
		slog.Info("BashTool: waiting for confirm", "id", confirmID, "cmd", args.Cmd)

		select {
		case approved := <-ch:
			if !approved {
				return "用户已拒绝执行此命令", nil
			}
		case <-time.After(120 * time.Second):
			return "等待用户确认超时（120s），命令未执行", nil
		case <-ctx.Done():
			return "任务已取消，命令未执行", nil
		}
	}

	return t.execute(ctx, args.Cmd, timeout)
}

// execute 实际执行 shell 命令，合并 stdout+stderr，实时推送输出。
func (t *BashTool) execute(ctx context.Context, cmd string, timeout time.Duration) (string, error) {
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := exec.CommandContext(execCtx, "sh", "-c", cmd)
	c.Dir = t.workDir

	stdout, err := c.StdoutPipe()
	if err != nil {
		return fmt.Sprintf("启动命令失败: %v", err), nil
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return fmt.Sprintf("启动命令失败: %v", err), nil
	}

	if err := c.Start(); err != nil {
		return fmt.Sprintf("启动命令失败: %v", err), nil
	}

	const maxOutput = 50 * 1024 // 50KB
	var buf bytes.Buffer
	combined := io.MultiReader(stdout, stderr)

	readBuf := make([]byte, 4096)
	for {
		n, readErr := combined.Read(readBuf)
		if n > 0 {
			chunk := string(readBuf[:n])
			if buf.Len() < maxOutput {
				buf.WriteString(chunk)
			}
			// 实时推送输出到前端
			if t.emitter != nil {
				t.emitter("bash_output", chunk, cmd, "")
			}
		}
		if readErr != nil {
			break
		}
	}

	_ = c.Wait()

	result := buf.String()
	if buf.Len() >= maxOutput {
		result += "\n[输出已截断，超过 50KB]"
	}
	if result == "" {
		result = "（命令执行完毕，无输出）"
	}
	return result, nil
}
