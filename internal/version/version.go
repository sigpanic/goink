// Package version 提供应用版本号，编译时通过 -ldflags 注入。
package version

import "runtime/debug"

// Version 由 Makefile 通过 -ldflags "-X github.com/sigpanic/goink/internal/version.Version=xxx" 注入。
// 未注入时默认为 "dev"。
var Version = "dev"

// CommitHash 由构建脚本通过 -ldflags 注入，格式为 8 位提交号及可选的 -dirty 后缀。
var CommitHash string

// BuildHash 返回构建时的短 Git 提交号；源码有未提交改动时附加 -dirty。
// 构建信息缺失时返回空字符串。
func BuildHash() string {
	if CommitHash != "" {
		return CommitHash
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return ""
	}
	if len(revision) > 8 {
		revision = revision[:8]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}
