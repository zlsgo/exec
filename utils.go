package exec

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/sohaha/zlsgo/zarray"
	"github.com/sohaha/zlsgo/zfile"
	"github.com/sohaha/zlsgo/zshell"
	"github.com/sohaha/zlsgo/zstring"
)

var tmpDir = zfile.TmpPath()

func init() {
	tmpDir = initTmpDir()
}

func initTmpDir() string {
	epath, err := os.Executable()
	if err != nil {
		epath = os.Args[0]
	}
	hash, _ := zstring.Md5File(epath)
	return zfile.RealPathMkdir(zfile.TmpPath() + "/" + hash)
}

func writeFile(path string, binary []byte) error {
	if len(binary) == 0 {
		return errors.New("binary is empty")
	}

	if err := zfile.WriteFile(path, binary); err != nil {
		return err
	}

	_ = os.Chmod(path, 0o755)
	return nil
}

func merge(e *Engine, ctx context.Context) (runCtx context.Context, cancel context.CancelFunc) {
	if ctx == nil {
		runCtx = e.ctx
		cancel = func() {}
	} else {
		runCtx, cancel = context.WithCancel(ctx)
		go func() {
			select {
			case <-e.ctx.Done():
				cancel()
			case <-ctx.Done():
				cancel()
			}
		}()
	}

	return
}

func (e *Engine) command(attr []string) (string, error) {
	if !zfile.FileExist(e.binFile) {
		return "", errors.New("BinFile not set")
	}

	args := append([]string{e.binFile}, zarray.Filter(attr, func(_ int, s string) bool {
		return s != ""
	})...)

	return joinCommand(args), nil
}

func joinCommand(args []string) string {
	return strings.Join(zarray.Map(args, func(_ int, arg string) string {
		return quoteCommandArg(arg)
	}), " ")
}

func quoteCommandArg(arg string) string {
	if arg == "" {
		return `""`
	}

	if strings.ContainsAny(arg, " \t\"'") {
		return strconv.Quote(arg)
	}

	return arg
}

var runCommand = func(ctx context.Context, command string, opt func(o *zshell.Options)) (code int, outStr string, errStr string, err error) {
	return zshell.RunContext(ctx, command, opt)
}

var callbackRunCommand = func(ctx context.Context, command string, callback func(str string, isStdout bool), opt func(o *zshell.Options)) (<-chan int, func(string), error) {
	return zshell.CallbackRunContext(ctx, command, callback, opt)
}
