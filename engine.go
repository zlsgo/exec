// Package exec provides a lightweight wrapper for preparing and running local,
// embedded, or downloaded binaries through a unified Go API.
package exec

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sohaha/zlsgo/zfile"
	"github.com/sohaha/zlsgo/zstring"
	"github.com/sohaha/zlsgo/ztype"
	"github.com/sohaha/zlsgo/zutil"
)

// Engine owns the lifecycle of one prepared binary, including optional extraction,
// download, execution, and cleanup of Engine-owned temporary files.
type Engine struct {
	options *Options
	tmpDir  string
	ctx     context.Context
	cancel  context.CancelFunc
	fs      embed.FS
	binFile string
	owned   bool
}

// New creates an Engine and prepares the target binary from one of three sources:
// an existing BinFile, embedded files in fs, or DownloadUrl.
func New(fs embed.FS, fn ...func(options *Options)) (*Engine, error) {
	ctx, cancel := context.WithCancel(context.Background())
	options := zutil.Optional(Options{}, fn...)
	e := &Engine{
		options: &options,
		ctx:     ctx,
		cancel:  cancel,
		fs:      fs,
	}

	if !zfile.FileExist(options.BinFile) {
		if err := toDisk(e); err != nil {
			return nil, err
		}
		e.binFile = filepath.Join(e.tmpDir, options.BinFile)
	} else {
		e.binFile = zfile.RealPath(options.BinFile)
		e.tmpDir = filepath.Dir(e.binFile)
	}

	return e, nil
}

// Dir returns the working directory that currently stores the prepared binary.
func (e *Engine) Dir() string {
	return e.tmpDir
}

func toDisk(e *Engine) error {
	e.tmpDir = filepath.Join(tmpDir, ztype.ToString(zstring.UUID()))
	e.owned = true
	total, err := embedToDisk(e.fs, e.tmpDir)
	if err != nil {
		return err
	}

	if total > 0 {
		return nil
	}

	return e.downloadToDisk()
}

// Run executes the prepared binary synchronously and returns exit code, stdout, stderr and error.
func (e *Engine) Run(ctx context.Context, attr []string, opt ...func(o *Options)) (code int, outStr string, errStr string, err error) {
	argv, err := e.argv(attr)
	if err != nil {
		return -1, "", "", err
	}

	runCtx, cancel := merge(e, ctx)
	defer cancel()

	return runCommand(runCtx, argv, e.runSpec(opt...))
}

// KeepRun executes the prepared binary in streaming mode.
// It returns a code channel and an input function for writing to stdin.
func (e *Engine) KeepRun(ctx context.Context, attr []string, callback func(str string, isStdout bool), opt ...func(o *Options)) (<-chan int, func(string), error) {
	argv, err := e.argv(attr)
	if err != nil {
		return nil, nil, err
	}

	runCtx, cancel := merge(e, ctx)
	codeCh, input, err := callbackRunCommand(runCtx, argv, callback, e.runSpec(opt...))
	if err != nil {
		cancel()
		return nil, nil, err
	}

	done := make(chan int, 1)
	go func() {
		defer cancel()
		code, ok := <-codeCh
		if ok {
			done <- code
		}
		close(done)
	}()

	return done, input, nil
}

// Release cancels the engine context and removes temporary files created by the current Engine.
// External BinFile targets are never deleted.
func (e *Engine) Release() error {
	e.cancel()

	if !e.owned || e.tmpDir == "" {
		return nil
	}

	if _, err := os.Stat(e.tmpDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := os.RemoveAll(e.tmpDir); err != nil {
		return fmt.Errorf("remove temp dir %s: %w", e.tmpDir, err)
	}

	return nil
}
