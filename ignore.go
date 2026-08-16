package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// shouldIgnore 判断路径是否命中忽略规则，返回原因便于排查配置。
func shouldIgnore(path string, cfg *Config) (bool, string) {
	// 扩展名匹配（不区分大小写）
	ext := strings.ToLower(filepath.Ext(path))
	for _, ignoreExt := range cfg.Monitor.Ignore.Extensions {
		if ext == strings.ToLower(ignoreExt) {
			return true, fmt.Sprintf("扩展名匹配: %s", ignoreExt)
		}
	}

	// 文件名模式匹配（仅对最后一级文件名生效）
	base := filepath.Base(path)
	for _, pattern := range cfg.Monitor.Ignore.Files {
		if matched, _ := filepath.Match(pattern, base); matched {
			return true, fmt.Sprintf("文件名匹配: %s", pattern)
		}
	}

	// 目录模式匹配
	for _, pattern := range cfg.Monitor.Ignore.Directories {
		if matchDirPattern(pattern, path) {
			return true, fmt.Sprintf("目录匹配: %s", pattern)
		}
	}

	return false, ""
}

// matchDirPattern 判断目录忽略规则是否命中：
//   - 绝对路径模式：忽略该目录及其全部子内容（前缀匹配）
//   - 相对名称/glob 模式：路径中任意一段命中即忽略（如 "temp" 可匹配 /var/www/temp/x.txt）
func matchDirPattern(pattern, path string) bool {
	pattern = filepath.Clean(pattern)
	if filepath.IsAbs(pattern) {
		rel, err := filepath.Rel(pattern, filepath.Clean(path))
		return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
	}

	for _, seg := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		if seg == "" || seg == "." {
			continue
		}
		if matched, _ := filepath.Match(pattern, seg); matched {
			return true
		}
	}
	return false
}
