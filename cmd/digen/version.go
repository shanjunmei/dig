package main

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// 通过 ldflags 注入版本信息（由 mage build 等构建工具设置）：
//
//	go build -ldflags "-X main.Version=v1.0.0 -X main.Commit=abc1234 -X main.BuildDate=2026-01-01T00:00:00Z"
//
// 当变量为空时（如用户使用 go install 安装），
// 会自动从运行时的 VCS 信息中读取版本号。
// Go 1.18+ 会将 VCS 信息嵌入二进制，debug.ReadBuildInfo() 可在运行时读取。
var (
	Version   string
	Commit    string
	BuildDate string
)

type resolvedVersion struct {
	version   string
	tag       string
	commit    string
	buildDate string
	commits   int
	dirty     bool
}

var cached *resolvedVersion

// resolveFromBuildInfo 从运行时的 debug.ReadBuildInfo() 解析 VCS 信息
func resolveFromBuildInfo() *resolvedVersion {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}

	ver := &resolvedVersion{}

	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		ver.version = info.Main.Version
	}

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			ver.commit = s.Value
		case "vcs.time":
			ver.buildDate = s.Value
		}
	}

	if strings.Contains(info.Main.Version, "+dirty") {
		ver.dirty = true
	}

	return ver
}

// getVersion 获取最终版本信息
// 优先使用 ldflags 注入的值，缺失时回退到 VCS 信息
func getVersion() *resolvedVersion {
	if cached != nil {
		return cached
	}

	// 先使用 ldflags 注入的值
	cached = &resolvedVersion{
		version:   Version,
		commit:    Commit,
		buildDate: BuildDate,
	}

	// 缺失值从 VCS 信息中获取
	if v := resolveFromBuildInfo(); v != nil {
		if cached.version == "" && v.version != "" {
			cached.version = v.version
		}
		if cached.commit == "" && v.commit != "" {
			cached.commit = v.commit
		}
		if cached.buildDate == "" && v.buildDate != "" {
			cached.buildDate = v.buildDate
		}
		if v.dirty {
			cached.dirty = true
		}
	}

	// 解析 git describe 格式的 version 字符串
	parseGitDescribe(cached)

	// 兼容 Go pseudo-version 的 +dirty 标记
	if strings.Contains(cached.version, "+dirty") {
		cached.dirty = true
	}

	return cached
}

// parseGitDescribe 解析 git describe --tags --dirty 输出格式
// 格式：<tag>[-<N>-g<hash>][-dirty]
//
//	v1.0.11                     → tag=v1.0.11
//	v1.0.11-dirty               → tag=v1.0.11, dirty=true
//	v1.0.11-13-g0f16d86         → tag=v1.0.11, commits=13, commit=0f16d86
//	v1.0.11-13-g0f16d86-dirty   → tag=v1.0.11, commits=13, commit=0f16d86, dirty=true
//
// 仅在 commit 为空时才从 version 字符串中提取短 hash。
func parseGitDescribe(v *resolvedVersion) {
	s := v.version

	// Go pseudo-version 由 resolveFromBuildInfo 处理
	if strings.Contains(s, "-0.") {
		return
	}

	if strings.HasSuffix(s, "-dirty") {
		v.dirty = true
		s = s[:len(s)-len("-dirty")]
	}

	// 提取 -g<hash>，仅在 commit 为空时使用
	if idx := strings.LastIndex(s, "-g"); idx >= 0 {
		hash := s[idx+2:]
		if len(hash) >= 4 {
			if v.commit == "" {
				v.commit = hash
			}
			s = s[:idx]
		}
	}

	// 提取 commit 数量（-<N>-）
	parts := strings.Split(s, "-")
	if len(parts) >= 2 {
		if n, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			v.commits = n
			s = strings.Join(parts[:len(parts)-1], "-")
		}
	}

	v.tag = s
}

// cleanVersion 提取干净的版本标签
func cleanVersion(v *resolvedVersion) string {
	if v.tag != "" {
		return v.tag
	}
	s := v.version
	if idx := strings.Index(s, "-0."); idx >= 0 {
		return s[:idx]
	}
	return s
}

// formatDate 格式化构建日期
func formatDate(s string) string {
	if s == "" {
		return ""
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05+00:00",
		"2006-01-02 15:04:05Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Local().Format("2006-01-02 15:04:05 -07:00")
		}
	}
	return s
}

func versionString() string {
	v := getVersion()

	var b strings.Builder

	if tag := cleanVersion(v); tag != "" {
		fmt.Fprintf(&b, "digen %s", tag)
		if v.commits > 0 {
			fmt.Fprintf(&b, "+%d", v.commits)
		}
	}

	if v.dirty {
		fmt.Fprintf(&b, " (dirty)")
	}
	fmt.Fprintln(&b)

	if v.commit != "" {
		fmt.Fprintf(&b, "  commit: %s\n", v.commit)
	}
	if date := formatDate(v.buildDate); date != "" {
		fmt.Fprintf(&b, "  built:  %s", date)
	}
	return b.String()
}
