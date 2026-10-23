[English](./README.EN.md) | 简体中文

![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/zlsgo/exec)
![flat](https://img.shields.io/github/languages/top/zlsgo/exec.svg?style=flat)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat)](https://pkg.go.dev/github.com/zlsgo/exec)

# exec

轻量的二进制执行封装，支持：

- 直接执行本地已有二进制
- 从 `embed.FS` 落盘后执行
- 从远程下载、缓存并处理二进制后执行

适合把命令行工具、辅助进程或嵌入式可执行文件封装成统一的 Go 调用入口。

## ✨ 特性

- **统一入口**：通过 `New` 创建 `Engine`，统一处理二进制准备、运行和释放
- **多种来源**：支持本地文件、`embed.FS`、`DownloadUrl`
- **可扩展下载处理**：兼容旧版 `DownloadHandler`，也支持带目标目录的 `DownloadHandlerWithDir`
- **运行控制**：支持环境变量、工作目录、同步执行和流式执行；参数按 argv 原样传递，不经 shell 二次解析
- **安全边界清晰**：外部 `BinFile` 不会在 `Release()` 时被误删
- **下载格式支持**：支持普通文件、`.zip`、`.tar.gz` / `.tgz` 和单文件 `.gz`，解压时保留硬链接与符号链接

## 🚀 快速开始

### 安装

```bash
go get github.com/zlsgo/exec
```

### 执行本地二进制

```go
package main

import (
	"context"
	"embed"
	"fmt"

	execbin "github.com/zlsgo/exec"
)

func main() {
	engine, err := execbin.New(embed.FS{}, func(options *execbin.Options) {
		options.BinFile = "/path/to/tool"
		options.Dir = "/path/to/workdir"
		options.Env = []string{"APP_MODE=prod"}
	})
	if err != nil {
		panic(err)
	}
	defer engine.Release()

	code, out, errOut, err := engine.Run(context.Background(), []string{"--version"})
	fmt.Println(code, out, errOut, err)
}
```

### 使用 embed 二进制

```go
package main

import (
	"context"
	"embed"

	execbin "github.com/zlsgo/exec"
)

//go:embed bin/*
var binFS embed.FS

func main() {
	engine, err := execbin.New(binFS, func(options *execbin.Options) {
		options.BinFile = "tool"
	})
	if err != nil {
		panic(err)
	}
	defer engine.Release()

	_, _, _, _ = engine.Run(context.Background(), []string{"--help"})
}
```

### 使用下载地址

```go
package main

import (
	"context"
	"embed"
	"fmt"

	execbin "github.com/zlsgo/exec"
)

func main() {
	engine, err := execbin.New(embed.FS{}, func(options *execbin.Options) {
		options.BinFile = "tool"
		options.DownloadUrl = "https://example.com/tool.zip"
		options.DownloadProgress = func(percent float64, size int64) {
			fmt.Printf("progress: %.2f%% size=%d\n", percent, size)
		}
	})
	if err != nil {
		panic(err)
	}
	defer engine.Release()

	_, _, _, _ = engine.Run(context.Background(), []string{"--help"})
}
```

### 下载后二次处理

如果下载产物不是最终可执行文件，可以通过 `DownloadHandlerWithDir` 自定义解包、重命名或安装逻辑。
回调参数分别是：

- `file`：下载缓存文件路径
- `dir`：当前 `Engine` 的目标落盘目录

```go
engine, err := execbin.New(embed.FS{}, func(options *execbin.Options) {
	options.BinFile = "tool"
	options.DownloadUrl = "https://example.com/tool.zip"
	options.DownloadHandlerWithDir = func(file string, dir string) error {
		// 在这里自行处理下载产物，例如解压到 dir、复制或重命名为目标文件
		return nil
	}
})
```

## 📦 主要接口

### `New`

创建执行引擎，并根据以下优先级准备二进制：

1. `BinFile` 指向的本地文件存在：直接使用
2. 本地文件不存在，但 `embed.FS` 中存在内容：落盘后使用
3. embed 内容为空，且配置了 `DownloadUrl`：下载后使用

### `Run`

同步执行命令，返回：

- 退出码
- 标准输出
- 标准错误
- 执行错误

### `KeepRun`

流式执行命令，适合长时间运行或需要实时消费输出的场景。

### `Release`

- 当二进制由当前 `Engine` 创建的临时目录提供时，会清理该目录
- 当 `BinFile` 指向外部已有文件时，只取消上下文，不删除外部目录

## ⚠️ 说明

- `BinFile` 既表示目标文件名，也可能表示已存在的绝对/相对路径
- `DownloadHandlerWithDir` 一旦配置，会优先接管下载产物处理，并负责把最终可执行文件放到当前 `Engine` 的目标目录
- 旧版 `DownloadHandler(file string) error` 仍然保留，用于兼容已有调用方
- 压缩包解压会校验目标路径，避免目录穿越
- 未自定义 handler 时，普通文件和单文件 `.gz` 会默认落盘到 `BinFile` 对应路径
- `Run` / `KeepRun` 会正确透传覆盖后的 `Env` 和 `Dir`
