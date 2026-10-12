package main

import "embed"

// Dist 是前端构建产物。just build 会先构建前端再编译 Go，测试与 vet 则用 frontend/dist/STUB 占位。
//
//go:embed all:frontend/dist
var Dist embed.FS
