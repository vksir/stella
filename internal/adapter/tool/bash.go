package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/vksir/stella/internal/core/agent"
)

const (
	// 超时上限（毫秒精度限制）。
	bashMaxTimeoutSeconds = 2147483647.0 / 1000
	// tail 保留的最大字节数，超出部分仅存在于完整输出文件。
	bashTailMaxBytes = DefaultMaxBytes * 2
)

// BashOptions 配置 bash 工具的运行环境。
type BashOptions struct {
	// Cwd 命令工作目录，默认进程当前目录。
	Cwd string
	// ShellPath 显式指定 bash 可执行文件路径。
	ShellPath string
	// CommandPrefix 附加到每次命令执行之前的初始化命令。
	CommandPrefix string
}

const bashDescription = "Execute a bash command in the current working directory. Returns stdout and stderr. Output is truncated to last 2000 lines or 50KB (whichever is hit first). If truncated, full output is saved to a temp file. Optionally provide a timeout in seconds."

var bashSchema = objectSchema(map[string]any{
	"command": strProp("Bash command to execute"),
	"timeout": numProp("Timeout in seconds (optional, no default timeout)"),
}, "command")

// bashTool 实现 bash 工具。
type bashTool struct {
	opts      BashOptions
	shell     string
	args      []string
	stdinMode bool // WSL bash 通过 stdin 传命令
	configErr error
}

// NewBash 创建 bash 工具。
func NewBash(options ...BashOptions) agent.Tool {
	bt := &bashTool{}
	if len(options) > 0 {
		bt.opts = options[0]
	}
	if bt.opts.Cwd == "" {
		bt.opts.Cwd, _ = os.Getwd()
	}
	bt.shell, bt.args, bt.stdinMode, bt.configErr = resolveShellConfig(bt.opts.ShellPath)
	return bt
}

func (bt *bashTool) Name() string { return "bash" }

func (bt *bashTool) Description() string { return bashDescription }

func (bt *bashTool) Schema() map[string]any { return bashSchema }

func (bt *bashTool) Execute(ctx context.Context, args string) (out agent.Content, err error) {
	defer func() {
		if err != nil {
			slog.Error("tool failed", "tool", "bash", "error", err)
		}
	}()
	if bt.configErr != nil {
		return nil, bt.configErr
	}
	if ctx.Err() != nil {
		return nil, errors.New("Command aborted")
	}
	var in struct {
		Command string          `json:"command"`
		Timeout json.RawMessage `json:"timeout"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return nil, err
	}
	timeoutText := ""
	var timeout float64
	if len(in.Timeout) > 0 && string(in.Timeout) != "null" {
		if err := json.Unmarshal(in.Timeout, &timeout); err != nil {
			return nil, err
		}
		timeoutText = string(in.Timeout)
		if timeout <= 0 {
			return nil, errors.New("Invalid timeout: must be a finite number of seconds")
		}
		if timeout > bashMaxTimeoutSeconds {
			return nil, fmt.Errorf("Invalid timeout: maximum is %v seconds", bashMaxTimeoutSeconds)
		}
	}
	if _, err := os.Stat(bt.opts.Cwd); err != nil {
		return nil, fmt.Errorf("Working directory does not exist: %s\nCannot execute bash commands.", bt.opts.Cwd)
	}

	command := in.Command
	if bt.opts.CommandPrefix != "" {
		command = bt.opts.CommandPrefix + "\n" + command
	}
	slog.Info("bash tool executed", "command", command, "timeout", timeoutText)
	raw := bt.runShell(ctx, command, timeout)

	outputText := raw.output
	if raw.truncation.Truncated {
		startLine := raw.truncation.TotalLines - raw.truncation.OutputLines + 1
		endLine := raw.truncation.TotalLines
		if raw.truncation.LastLinePartial {
			lastLineSize := formatSize(raw.lastLineBytes)
			outputText += fmt.Sprintf("\n\n[Showing last %s of line %d (line is %s). Full output: %s]",
				formatSize(raw.truncation.OutputBytes), endLine, lastLineSize, raw.fullOutputPath)
		} else if raw.truncation.TruncatedBy == "lines" {
			outputText += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Full output: %s]",
				startLine, endLine, raw.truncation.TotalLines, raw.fullOutputPath)
		} else {
			outputText += fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Full output: %s]",
				startLine, endLine, raw.truncation.TotalLines, formatSize(DefaultMaxBytes), raw.fullOutputPath)
		}
	}

	appendStatus := func(status string) string {
		if outputText == "" {
			return status
		}
		return outputText + "\n\n" + status
	}
	if raw.cancelled {
		return nil, errors.New(appendStatus("Command aborted"))
	}
	if raw.timedOut {
		return nil, errors.New(appendStatus(fmt.Sprintf("Command timed out after %s seconds", timeoutText)))
	}
	if raw.execErr != nil {
		return nil, raw.execErr
	}
	if raw.exitCode != 0 {
		return nil, errors.New(appendStatus(fmt.Sprintf("Command exited with code %d", raw.exitCode)))
	}
	if outputText == "" {
		outputText = "(no output)"
	}
	return agent.TextContent(outputText), nil
}

// shellRunResult 是一次命令执行的结果。
type shellRunResult struct {
	output         string
	truncation     TruncationResult
	fullOutputPath string
	lastLineBytes  int
	exitCode       int
	cancelled      bool
	timedOut       bool
	execErr        error
}

// runShell 执行命令并累积输出。
func (bt *bashTool) runShell(ctx context.Context, command string, timeout float64) shellRunResult {
	st := &captureState{accepting: true}
	w := &captureWriter{st: st}

	args := bt.args
	if !bt.stdinMode {
		args = append(args, command)
	}
	cmd := exec.Command(bt.shell, args...)
	configureShellProcess(cmd)
	cmd.WaitDelay = time.Second
	cmd.Dir = bt.opts.Cwd
	cmd.Env = os.Environ()
	cmd.Stdout, cmd.Stderr = w, w
	if bt.stdinMode {
		cmd.Stdin = strings.NewReader(command)
	}

	execDone := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		execDone <- cmd.Run()
		close(finished)
	}()

	var timedOut atomic.Bool
	var timer *time.Timer
	if timeout > 0 {
		timer = time.AfterFunc(time.Duration(timeout*float64(time.Second)), func() {
			select {
			case <-finished:
				return
			default:
				timedOut.Store(true)
				killProcessTree(cmd)
			}
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			killProcessTree(cmd)
		case <-finished:
		}
	}()
	runErr := <-execDone
	if timer != nil {
		timer.Stop()
	}

	st.accepting = false
	st.finish()
	progress := st.createProgress()

	result := shellRunResult{
		output:         progress.output,
		truncation:     progress.truncation,
		fullOutputPath: progress.fullOutputPath,
		lastLineBytes:  progress.lastLineBytes,
	}
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		result.cancelled = true
	case timedOut.Load():
		result.timedOut = true
	case st.captureErr != nil:
		result.execErr = fmt.Errorf("failed to save bash output: %w", st.captureErr)
	case errors.Is(runErr, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success():
		result.exitCode = 0
	case errors.As(runErr, &exitErr):
		result.exitCode = exitErr.ExitCode()
	case runErr != nil:
		result.execErr = runErr
	}
	return result
}

// resolveShellConfig 按平台解析 shell：
// 显式 shellPath > Windows 常见 Git Bash 位置 > PATH 中的 bash > Unix /bin/bash。
func resolveShellConfig(shellPath string) (shell string, args []string, stdinMode bool, err error) {
	getConfig := func(s string) (string, []string, bool) {
		if isLegacyWslBashPath(s) {
			return s, []string{"-s"}, true
		}
		return s, []string{"-c"}, false
	}
	if shellPath != "" {
		if _, statErr := os.Stat(shellPath); statErr != nil {
			return "", nil, false, fmt.Errorf("Custom shell path not found: %s", shellPath)
		}
		shell, args, stdinMode = getConfig(shellPath)
		return shell, args, stdinMode, nil
	}

	if runtime.GOOS == "windows" {
		var paths []string
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			paths = append(paths, filepath.Join(pf, "Git", "bin", "bash.exe"))
		}
		if pf86 := os.Getenv("ProgramFiles(x86)"); pf86 != "" {
			paths = append(paths, filepath.Join(pf86, "Git", "bin", "bash.exe"))
		}
		for _, p := range paths {
			if _, statErr := os.Stat(p); statErr == nil {
				shell, args, stdinMode = getConfig(p)
				return shell, args, stdinMode, nil
			}
		}
		if bashPath, lookErr := exec.LookPath("bash.exe"); lookErr == nil {
			shell, args, stdinMode = getConfig(bashPath)
			return shell, args, stdinMode, nil
		}
		return "", nil, false, fmt.Errorf(
			"No bash shell found. Options:\n"+
				"  1. Install Git for Windows: https://git-scm.com/download/win\n"+
				"  2. Add your bash to PATH (Cygwin, MSYS2, etc.)\n"+
				"  3. Set shellPath in settings.json\n\n"+
				"Searched Git Bash in:\n  %s", strings.Join(paths, "\n  "))
	}

	if _, statErr := os.Stat("/bin/bash"); statErr == nil {
		shell, args, stdinMode = getConfig("/bin/bash")
		return shell, args, stdinMode, nil
	}
	if bashPath, lookErr := exec.LookPath("bash"); lookErr == nil {
		shell, args, stdinMode = getConfig(bashPath)
		return shell, args, stdinMode, nil
	}
	return "sh", []string{"-c"}, false, nil
}

var wslBashRe = regexp.MustCompile(`^[a-z]:\\windows\\(system32|sysnative)\\bash\.exe$`)

// isLegacyWslBashPath 判断是否为 WSL 的 bash.exe（需通过 stdin 传命令）。
func isLegacyWslBashPath(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "/", "\\"))
	return wslBashRe.MatchString(normalized)
}

// captureProgress 是输出累积的进度快照。
type captureProgress struct {
	output         string
	truncation     TruncationResult
	fullOutputPath string
	lastLineBytes  int
}

// captureState 累积命令 stdout/stderr 输出。
type captureState struct {
	mu                  sync.Mutex
	accepting           bool
	tail                []byte // 保留最后 bashTailMaxBytes 字节
	totalBytes          int
	completedLines      int
	hasOpenLine         bool
	currentLineBytes    int
	pendingUTF8         []byte
	rawChunks           [][]byte
	totalRawBytes       int
	fullOutput          *os.File
	fullOutputPath      string
	fullOutputRequested bool
	captureErr          error
}

// captureWriter 串行化两个输出流的写入。
type captureWriter struct {
	st *captureState
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.st.mu.Lock()
	defer w.st.mu.Unlock()
	if w.st.accepting {
		w.st.onChunk(p)
	}
	return len(p), nil
}

// onChunk 处理一段输出：保留原始字节，并流式解码展示文本。
func (st *captureState) onChunk(chunk []byte) {
	st.totalRawBytes += len(chunk)
	if st.fullOutputRequested {
		st.appendFullOutput(chunk)
	} else {
		st.rawChunks = append(st.rawChunks, append([]byte(nil), chunk...))
	}

	data := append(st.pendingUTF8, chunk...)
	st.pendingUTF8 = nil
	var decoded strings.Builder
	i := 0
	for i < len(data) && utf8.FullRune(data[i:]) {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			decoded.WriteRune(utf8.RuneError)
		} else {
			decoded.Write(data[i : i+size])
		}
		i += size
	}
	st.pendingUTF8 = append(st.pendingUTF8, data[i:]...)
	st.appendDecodedText(sanitizeBinaryOutput(decoded.String()))
	totalLines := st.completedLines
	if st.hasOpenLine {
		totalLines++
	}
	if (st.totalRawBytes > DefaultMaxBytes || st.totalBytes > DefaultMaxBytes || totalLines > DefaultMaxLines) && !st.fullOutputRequested {
		st.ensureFullOutputFile()
	}
}

func (st *captureState) appendDecodedText(text string) {
	st.totalBytes += len(text)
	st.completedLines += strings.Count(text, "\n")
	if lastNewline := strings.LastIndex(text, "\n"); lastNewline >= 0 {
		st.currentLineBytes = len(text[lastNewline+1:])
		st.hasOpenLine = st.currentLineBytes > 0
	} else if text != "" {
		st.currentLineBytes += len(text)
		st.hasOpenLine = true
	}
	st.tail = []byte(trimToLastUTF8Bytes(string(append(st.tail, text...)), bashTailMaxBytes))
}

// sanitizeBinaryOutput 过滤不可打印的控制字符，保留 \t \n \r。
func sanitizeBinaryOutput(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == 0x09 || r == 0x0A || r == 0x0D:
			return r
		case r <= 0x1F:
			return -1
		case r >= 0xFFF9 && r <= 0xFFFB:
			return -1
		default:
			return r
		}
	}, s)
}

// ensureFullOutputFile 创建完整输出临时文件并写入当前已累积的原始字节。
func (st *captureState) ensureFullOutputFile() {
	if st.fullOutputRequested || st.captureErr != nil {
		return
	}
	st.fullOutputRequested = true
	file, err := os.CreateTemp("", "pi-bash-*.log")
	if err != nil {
		st.captureErr = err
		return
	}
	st.fullOutput = file
	st.fullOutputPath = file.Name()
	for _, chunk := range st.rawChunks {
		st.appendFullOutput(chunk)
	}
	st.rawChunks = nil
}

// appendFullOutput 追加一段原始输出到完整输出文件。
func (st *captureState) appendFullOutput(chunk []byte) {
	if st.fullOutput == nil || st.captureErr != nil {
		return
	}
	if _, err := st.fullOutput.Write(chunk); err != nil {
		st.captureErr = err
	}
}

// finish 结束输出累积并关闭完整输出文件。
func (st *captureState) finish() {
	if len(st.pendingUTF8) > 0 {
		st.appendDecodedText(sanitizeBinaryOutput(strings.ToValidUTF8(string(st.pendingUTF8), "\uFFFD")))
		st.pendingUTF8 = nil
	}
	progress := st.createProgress()
	if progress.truncation.Truncated && !st.fullOutputRequested {
		st.ensureFullOutputFile()
	}
	if st.fullOutput != nil {
		if err := st.fullOutput.Close(); err != nil && st.captureErr == nil {
			st.captureErr = err
		}
		st.fullOutput = nil
	}
}

// createProgress 生成当前进度快照。
func (st *captureState) createProgress() captureProgress {
	tailTruncation := truncateTail(string(st.tail), DefaultMaxLines, DefaultMaxBytes)
	totalLines := st.completedLines
	if st.hasOpenLine {
		totalLines++
	}
	truncated := totalLines > DefaultMaxLines || st.totalBytes > DefaultMaxBytes
	truncatedBy := tailTruncation.TruncatedBy
	if truncated && truncatedBy == "" {
		if st.totalBytes > DefaultMaxBytes {
			truncatedBy = "bytes"
		} else {
			truncatedBy = "lines"
		}
	}
	truncation := tailTruncation
	truncation.Truncated = truncated
	truncation.TruncatedBy = truncatedBy
	truncation.TotalLines = totalLines
	truncation.TotalBytes = st.totalBytes

	output := string(st.tail)
	if truncated {
		output = truncation.Content
	}
	return captureProgress{
		output:         output,
		truncation:     truncation,
		fullOutputPath: st.fullOutputPath,
		lastLineBytes:  st.currentLineBytes,
	}
}
