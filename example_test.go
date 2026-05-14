package exec_test

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	execbin "github.com/zlsgo/exec"
)

func ExampleNew_localBinary() {
	binFile, cleanup := writeExampleScript()
	defer cleanup()

	engine, err := execbin.New(embed.FS{}, func(options *execbin.Options) {
		options.BinFile = binFile
	})
	if err != nil {
		panic(err)
	}
	defer engine.Release()

	code, out, _, err := engine.Run(context.Background(), []string{"example"})
	if err != nil {
		panic(err)
	}

	fmt.Println(code, strings.TrimSpace(out))
	// Output:
	// 0 local-example
}

func ExampleOptions_DownloadHandlerWithDir() {
	_ = execbin.Options{
		BinFile:     "tool",
		DownloadUrl: "https://example.com/tool.gz",
		DownloadHandlerWithDir: func(file string, dir string) error {
			fmt.Println(filepath.Base(file) != "", dir != "")
			return nil
		},
	}

	fmt.Println(true)
	// Output:
	// true
}

func writeExampleScript() (string, func()) {
	dir, err := os.MkdirTemp("", "exec-example-*")
	if err != nil {
		panic(err)
	}

	var path string
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, "example.cmd")
		err = os.WriteFile(path, []byte("@echo off\r\necho local-example\r\n"), 0o755)
	} else {
		path = filepath.Join(dir, "example.sh")
		err = os.WriteFile(path, []byte("#!/bin/sh\nprintf 'local-example\\n'\n"), 0o755)
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		panic(err)
	}

	return path, func() {
		_ = os.RemoveAll(dir)
	}
}
