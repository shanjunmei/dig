package main

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

// 通过 ldflags 注入版本信息：
//
//	go build -ldflags "-X main.Version=v1.0.0 -X main.Commit=abc1234 -X main.BuildDate=2026-01-01T00:00:00Z"
//
// 当用户使用 go install 安装时，无法注入 ldflags，此时会自动从运行时的 VCS 信息中读取版本号。
// Go 1.18+ 会将 VCS 信息嵌入二进制，debug.ReadBuildInfo() 可在运行时读取。
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

type resolvedVersion struct {
	version   string
	commit    string
	buildDate string
}

var cached *resolvedVersion

// isFlagDefault 判断变量是否为 ldflags 未注入的默认值
func isFlagDefault(v, def string) bool {
	return v == def
}

// resolveFromBuildInfo 从运行时的 debug.ReadBuildInfo() 解析 VCS 信息
// Go 1.18+ 将模块版本和 VCS 信息嵌入二进制，可直接读取
func resolveFromBuildInfo() *resolvedVersion {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}

	ver := &resolvedVersion{
		version:   Version,
		commit:    Commit,
		buildDate: BuildDate,
	}

	// 模块版本（go install module@vX.Y.Z 时为标签名；本地构建时为 "(devel)"）
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		ver.version = info.Main.Version
	}

	// 从 Settings 中提取 VCS 信息
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			ver.commit = s.Value
		case "vcs.time":
			ver.buildDate = s.Value
		}
	}

	return ver
}

// getVersion 获取最终版本信息（优先使用 ldflags，回退到运行时 VCS 信息）
func getVersion() *resolvedVersion {
	if cached != nil {
		return cached
	}
	cached = &resolvedVersion{
		version:   Version,
		commit:    Commit,
		buildDate: BuildDate,
	}

	// 仅在 ldflags 未注入时尝试读取运行时 VCS 信息
	if isFlagDefault(Version, "dev") || isFlagDefault(Commit, "none") || isFlagDefault(BuildDate, "unknown") {
		if v := resolveFromBuildInfo(); v != nil {
			if isFlagDefault(Version, "dev") && v.version != "" {
				cached.version = v.version
			}
			if isFlagDefault(Commit, "none") && v.commit != "" {
				cached.commit = v.commit
			}
			if isFlagDefault(BuildDate, "unknown") && v.buildDate != "" {
				cached.buildDate = v.buildDate
			}
		}
	}
	return cached
}

// shortCommit 将 commit hash 截短到 8 个字符
func shortCommit(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// cleanVersion 从 pseudo-version 中提取干净的 semver 标签
// 例如 "v1.0.12-0.20260729062747-c51f61a8a195+dirty" → "v1.0.12"
func cleanVersion(v string) string {
	if idx := strings.Index(v, "-"); idx > 0 {
		return v[:idx]
	}
	return v
}

// versionNote 提取 pseudo-version 中的附加信息（dirty、pre-release 等）
func versionNote(v string) string {
	note := ""
	if strings.Contains(v, "+dirty") {
		note += " (dirty)"
	}
	return note
}

// formatDate 格式化构建日期，支持 ISO 8601 和 RFC3339 输入
func formatDate(s string) string {
	if s == "unknown" {
		return s
	}
	// 尝试 RFC3339 / ISO8601 格式
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05+00:00",
		"2006-01-02 15:04:05Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			// 转为本地时区显示
			return t.Local().Format("2006-01-02 15:04:05 -07:00")
		}
	}
	// 无法解析则原样返回
	return s
}

func versionString() string {
	v := getVersion()
	cleanVer := cleanVersion(v.version)
	note := versionNote(v.version)
	commit := shortCommit(v.commit)
	date := formatDate(v.buildDate)

	var b strings.Builder
	fmt.Fprintf(&b, "digen %s%s\n", cleanVer, note)
	fmt.Fprintf(&b, "  commit: %s\n", commit)
	fmt.Fprintf(&b, "  built:  %s", date)
	return b.String()
}
