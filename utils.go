package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/sohaha/zlsgo/zfile"
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
	return zfile.RealPathMkdir(filepath.Join(zfile.TmpPath(), hash))
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

// argv returns the exact argument vector for the prepared binary.
// Empty arguments are dropped, matching the historical behaviour.
func (e *Engine) argv(attr []string) ([]string, error) {
	if !zfile.FileExist(e.binFile) {
		return nil, errors.New("BinFile not set")
	}

	args := make([]string, 1, len(attr)+1)
	args[0] = e.binFile
	for _, arg := range attr {
		if arg != "" {
			args = append(args, arg)
		}
	}

	return args, nil
}
