package tool

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// 归一化路径中的 Unicode 空格。
const narrowNoBreakSpace = "\u202F"

var (
	unicodeSpaceRe = regexp.MustCompile("[\u00A0\u2000-\u200A\u202F\u205F\u3000]")
	amPMRe         = regexp.MustCompile(`(?i) (AM|PM)\.`)
)

// normalizeToolPath 归一化模型传入的路径：将 Unicode 空格替换为普通空格，剥离 "@" 前缀。
func normalizeToolPath(path string) string {
	normalized := unicodeSpaceRe.ReplaceAllString(path, " ")
	if strings.HasPrefix(normalized, "@") {
		return normalized[1:]
	}
	return normalized
}

// resolveToolPath 将路径解析为绝对路径。
func resolveToolPath(path string) (string, error) {
	path = normalizeToolPath(path)
	if path == "~" || strings.HasPrefix(path, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(path, `~\`)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimLeft(path[1:], `/\`))
	}
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil {
			return "", err
		}
		if u.Host != "" && u.Host != "localhost" {
			if runtime.GOOS != "windows" {
				return "", errors.New("file URL host is not local")
			}
			path = `\\` + u.Host + filepath.FromSlash(u.Path)
		} else {
			path = u.Path
			if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
				path = path[1:]
			}
		}
	}
	if runtime.GOOS == "windows" {
		path = normalizeWindowsShellPath(path)
	}
	return filepath.Abs(path)
}

func normalizeWindowsShellPath(path string) string {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, `\`) {
		return path
	}
	for _, prefix := range []string{"/mnt/", "/cygdrive/", "/"} {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		rest := strings.TrimPrefix(path, prefix)
		if len(rest) > 0 && ((rest[0] >= 'a' && rest[0] <= 'z') || (rest[0] >= 'A' && rest[0] <= 'Z')) && (len(rest) == 1 || rest[1] == '/') {
			return strings.ToUpper(rest[:1]) + `:\` + strings.ReplaceAll(strings.TrimPrefix(rest[1:], "/"), "/", `\`)
		}
	}
	return path
}

// resolveReadToolPath 解析读取路径，并尝试常见 Unicode 变体。
func resolveReadToolPath(path string) (string, error) {
	resolved, err := resolveToolPath(path)
	if err != nil {
		return "", err
	}
	nfd := norm.NFD.String(resolved)
	variants := []string{
		resolved,
		amPMRe.ReplaceAllString(resolved, narrowNoBreakSpace+"$1."),
		nfd,
		strings.ReplaceAll(resolved, "'", "\u2019"),
		strings.ReplaceAll(nfd, "'", "\u2019"),
	}
	seen := make(map[string]bool)
	for _, variant := range variants {
		if seen[variant] {
			continue
		}
		seen[variant] = true
		if _, err := os.Stat(variant); err == nil {
			return variant, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return resolved, nil
}
