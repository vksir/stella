package toolx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	return filepath.Abs(normalizeToolPath(path))
}

// resolveReadToolPath 解析读取路径，并尝试常见 Unicode 变体；NFD 变体仅适用于 macOS 场景，未实现。
func resolveReadToolPath(path string) (string, error) {
	resolved, err := resolveToolPath(path)
	if err != nil {
		return "", err
	}
	variants := []string{
		resolved,
		amPMRe.ReplaceAllString(resolved, narrowNoBreakSpace+"$1."),
		strings.ReplaceAll(resolved, "'", "\u2019"),
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
