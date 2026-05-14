English | [简体中文](./README.md)

![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/zlsgo/exec)
![flat](https://img.shields.io/github/languages/top/zlsgo/exec.svg?style=flat)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat)](https://pkg.go.dev/github.com/zlsgo/exec)

# exec

A lightweight binary execution wrapper that supports:

- Running an existing local binary
- Extracting and running a binary from `embed.FS`
- Downloading, caching, processing, and then running a binary

It is useful when you want to wrap CLI tools, helper processes, or embedded executables behind a unified Go API.

## ✨ Features

- **Unified entry**: create an `Engine` with `New` and manage prepare/run/release in one place
- **Multiple sources**: supports local files, `embed.FS`, and `DownloadUrl`
- **Custom download handling**: keeps legacy `DownloadHandler` compatibility and also supports `DownloadHandlerWithDir`
- **Runtime control**: supports environment variables, working directory, sync run, and streaming run
- **Clear safety boundary**: external `BinFile` targets are not deleted by `Release()`
- **Download format support**: supports plain files, `.zip`, `.tar.gz` / `.tgz`, and single-file `.gz`

## 🚀 QuickStart

### Install

```bash
go get github.com/zlsgo/exec
```

### Run a local binary

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

### Run a binary from embed

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

### Use a download URL

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

### Post-process downloaded files

If the downloaded artifact is not the final executable, use `DownloadHandlerWithDir` for custom unpacking, renaming, or install logic.
The callback arguments are:

- `file`: downloaded cache file path
- `dir`: target directory for the current `Engine`

```go
engine, err := execbin.New(embed.FS{}, func(options *execbin.Options) {
	options.BinFile = "tool"
	options.DownloadUrl = "https://example.com/tool.zip"
	options.DownloadHandlerWithDir = func(file string, dir string) error {
		// Custom handling for the downloaded artifact, then place the final binary into dir
		return nil
	}
})
```

## 📦 Main API

### `New`

Creates an execution engine and prepares the binary in the following priority order:

1. `BinFile` points to an existing local file: use it directly
2. Local file is missing, but `embed.FS` contains content: extract and use it
3. Embed content is empty and `DownloadUrl` is configured: download and use it

### `Run`

Runs the command synchronously and returns:

- exit code
- stdout
- stderr
- execution error

### `KeepRun`

Runs the command in streaming mode, suitable for long-running processes or real-time output consumption.

### `Release`

- Cleans the temporary directory only when the binary was prepared by the current `Engine`
- Cancels context only when `BinFile` points to an external existing file

## ⚠️ Notes

- `BinFile` can be either a target file name or an existing absolute/relative path
- Once configured, `DownloadHandlerWithDir` takes priority for downloaded artifacts and is responsible for placing the final executable into the current Engine target directory
- The legacy `DownloadHandler(file string) error` is still supported for backward compatibility
- Archive extraction validates destination paths to avoid path traversal
- Without a custom handler, plain files and single-file `.gz` archives are written to the `BinFile` target path by default
- `Run` and `KeepRun` correctly apply overridden `Env` and `Dir`
