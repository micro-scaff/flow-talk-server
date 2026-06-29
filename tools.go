//go:build tools

// Package tools 记录本项目使用的本地开发工具依赖。
// 这些工具不会被编进服务二进制，但 go mod tidy 会保留它们，方便统一安装版本。
package tools

import (
	// fresh 是开发期热重载工具。
	// 这里使用空导入让 go mod tidy 保留版本，实际运行仍然通过 `fresh` 命令启动。
	_ "github.com/pilu/fresh"
)
