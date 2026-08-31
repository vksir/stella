package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func argsOf(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func TestSmokeWriteReadEdit(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// write 自动创建父目录
	w := NewWrite()
	out, err := w.Execute(ctx, argsOf(map[string]any{"path": filepath.Join(dir, "nested", "dir", "file.txt"), "content": "hello"}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text() != "Successfully wrote to "+filepath.Join(dir, "nested", "dir", "file.txt") {
		t.Fatalf("write: %q", out)
	}

	r := NewRead()
	out, err = r.Execute(ctx, argsOf(map[string]any{"path": filepath.Join(dir, "nested", "dir", "file.txt")}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text() != "hello" {
		t.Fatalf("read: %q", out)
	}

	// read offset/limit：66 行文件中 offset=51 limit=10
	var sb strings.Builder
	for i := 1; i <= 66; i++ {
		sb.WriteString(strings.Repeat("x", 20))
		sb.WriteByte('\n')
	}
	big := filepath.Join(dir, "big.txt")
	os.WriteFile(big, []byte(sb.String()), 0o644)
	out, err = r.Execute(ctx, argsOf(map[string]any{"path": big, "offset": 51, "limit": 10}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.Text(), "\n\n[7 more lines in file. Use offset=61 to continue.]") {
		t.Fatalf("read limit: %q", out)
	}

	// read 行数截断：2500 行
	sb.Reset()
	for i := 0; i < 2500; i++ {
		sb.WriteString("line\n")
	}
	bigLines := filepath.Join(dir, "biglines.txt")
	os.WriteFile(bigLines, []byte(sb.String()), 0o644)
	out, err = r.Execute(ctx, argsOf(map[string]any{"path": bigLines}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text(), "[Showing lines 1-2000 of 2501. Use offset=2001 to continue.]") {
		t.Fatalf("read truncation: %q", out)
	}

	// read 图片检测（PNG 签名）
	pngFile := filepath.Join(dir, "img.png")
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(pngFile, pngData.Bytes(), 0o644)
	out, err = r.Execute(ctx, argsOf(map[string]any{"path": pngFile}))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Text != "Read image file [image/png]" || out[1].Type != "image" || out[1].MimeType != "image/png" {
		t.Fatalf("read image: %v", out)
	}

	// read offset 越界
	_, err = r.Execute(ctx, argsOf(map[string]any{"path": big, "offset": 999}))
	if err == nil || !strings.Contains(err.Error(), "Offset 999 is beyond end of file") {
		t.Fatalf("read offset: %v", err)
	}

	// edit 多块替换
	e := NewEdit()
	editFile := filepath.Join(dir, "edit.txt")
	os.WriteFile(editFile, []byte("alpha\nbeta\ngamma\n"), 0o644)
	out, err = e.Execute(ctx, argsOf(map[string]any{
		"path":  editFile,
		"edits": []map[string]any{{"oldText": "alpha", "newText": "A"}, {"oldText": "gamma", "newText": "G"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text() != "Successfully replaced 2 block(s) in "+editFile+"." {
		t.Fatalf("edit: %q", out)
	}
	content, _ := os.ReadFile(editFile)
	if string(content) != "A\nbeta\nG\n" {
		t.Fatalf("edit content: %q", content)
	}

	// edit duplicate：单个 oldText 出现多次
	os.WriteFile(editFile, []byte("beta\nbeta\n"), 0o644)
	_, err = e.Execute(ctx, argsOf(map[string]any{
		"path":  editFile,
		"edits": []map[string]any{{"oldText": "beta", "newText": "x"}},
	}))
	if err == nil || !strings.Contains(err.Error(), "Found 2 occurrences of the text in") {
		t.Fatalf("edit duplicate: %v", err)
	}

	// edit overlap
	os.WriteFile(editFile, []byte("beta\n"), 0o644)
	_, err = e.Execute(ctx, argsOf(map[string]any{
		"path":  editFile,
		"edits": []map[string]any{{"oldText": "beta", "newText": "B"}, {"oldText": "bet", "newText": "x"}},
	}))
	if err == nil || !strings.Contains(err.Error(), "overlap in") {
		t.Fatalf("edit overlap: %v", err)
	}

	// edit not found
	_, err = e.Execute(ctx, argsOf(map[string]any{"path": editFile, "edits": []map[string]any{{"oldText": "nope", "newText": "x"}}}))
	if err == nil || !strings.Contains(err.Error(), "Could not find the exact text in") {
		t.Fatalf("edit not found: %v", err)
	}

	// edit legacy 顶层 oldText/newText
	out, err = e.Execute(ctx, argsOf(map[string]any{"path": editFile, "oldText": "beta", "newText": "B"}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text() != "Successfully replaced 1 block(s) in "+editFile+"." {
		t.Fatalf("edit legacy: %q", out)
	}

	// edit fuzzy 匹配（全角括号/弯引号/长横线归一化）
	os.WriteFile(editFile, []byte("x = \uFF08y\uFF09; z \u2014 w\n"), 0o644)
	out, err = e.Execute(ctx, argsOf(map[string]any{
		"path":  editFile,
		"edits": []map[string]any{{"oldText": "x = (y); z - w", "newText": "ok"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(editFile)
	if string(content) != "ok\n" {
		t.Fatalf("edit fuzzy content: %q", content)
	}

	// edit 不存在文件
	_, err = e.Execute(ctx, argsOf(map[string]any{"path": filepath.Join(dir, "nope.txt"), "edits": []map[string]any{{"oldText": "a", "newText": "b"}}}))
	if err == nil || !strings.Contains(err.Error(), "Error code: not_found") {
		t.Fatalf("edit not found file: %v", err)
	}

	// edit 保留 CRLF 行尾
	crlfFile := filepath.Join(dir, "crlf.txt")
	os.WriteFile(crlfFile, []byte("one\r\ntwo\r\n"), 0o644)
	out, err = e.Execute(ctx, argsOf(map[string]any{
		"path":  crlfFile,
		"edits": []map[string]any{{"oldText": "two", "newText": "TWO"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(crlfFile)
	if string(content) != "one\r\nTWO\r\n" {
		t.Fatalf("edit crlf content: %q", content)
	}
}

func TestSmokeBash(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx := context.Background()
	b := NewBash()

	out, err := b.Execute(ctx, argsOf(map[string]any{"command": "echo hello"}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text() != "hello\n" {
		t.Fatalf("bash echo: %q", out)
	}

	_, err = b.Execute(ctx, argsOf(map[string]any{"command": "exit 7"}))
	if err == nil || !strings.Contains(err.Error(), "Command exited with code 7") {
		t.Fatalf("bash exit code: %v", err)
	}

	_, err = b.Execute(ctx, argsOf(map[string]any{"command": "sleep 5", "timeout": 0.05}))
	if err == nil || !strings.Contains(err.Error(), "Command timed out after 0.05 seconds") {
		t.Fatalf("bash timeout: %v", err)
	}

	// 截断 2500 行输出
	cmd := "for i in $(seq 1 2500); do echo line$i; done"
	out, err = b.Execute(ctx, argsOf(map[string]any{"command": cmd}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text(), "[Showing lines 501-2500 of 2500. Full output:") {
		t.Fatalf("bash truncation: %q", out)
	}
	if !strings.Contains(out.Text(), "line2500") {
		t.Fatalf("bash truncation tail: %q", out)
	}

	// 超时校验错误
	_, err = b.Execute(ctx, argsOf(map[string]any{"command": "echo x", "timeout": 0}))
	if err == nil || !strings.Contains(err.Error(), "Invalid timeout") {
		t.Fatalf("bash bad timeout: %v", err)
	}

	// 空输出
	out, err = b.Execute(ctx, argsOf(map[string]any{"command": "true"}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text() != "(no output)" {
		t.Fatalf("bash no output: %q", out)
	}
}
