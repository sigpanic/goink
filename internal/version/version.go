// Package version 提供应用版本号，编译时通过 -ldflags 注入。
package version

// Version 由 Makefile 通过 -ldflags "-X github.com/sigpanic/goink/internal/version.Version=xxx" 注入。
// 未注入时默认为 "dev"。
var Version = "dev"
