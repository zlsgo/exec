package exec

import (
	"github.com/sohaha/zlsgo/zfile"
	"github.com/sohaha/zlsgo/zshell"
	"github.com/sohaha/zlsgo/zutil"
)

// Options controls how Engine resolves, prepares and runs a binary.
type Options struct {
	// BinFile is either an existing binary path or the target file name after extraction/download.
	BinFile string
	// DownloadHandler keeps backward compatibility for callers that only need the downloaded file path.
	DownloadHandler func(file string) error
	// DownloadHandlerWithDir receives both the downloaded file path and the Engine target directory.
	DownloadHandlerWithDir func(file string, dir string) error
	// DownloadProgress reports current download percentage and total size in bytes.
	DownloadProgress func(percent float64, size int64)
	// DownloadUrl is used when BinFile cannot be resolved locally or from embed.FS.
	DownloadUrl string
	// Env overrides process environment variables for Run and KeepRun.
	Env []string
	// Dir overrides the working directory for Run and KeepRun.
	Dir string
}

// BinFile returns the resolved binary path used by the current Engine instance.
func (e *Engine) BinFile() string {
	return zfile.RealPath(e.binFile)
}

func (e *Engine) wrapOptions(o ...func(o *Options)) func(o *zshell.Options) {
	opt := zutil.Optional(Options{Env: e.options.Env, Dir: e.options.Dir}, o...)

	return func(o *zshell.Options) {
		if opt.Env != nil {
			o.Env = opt.Env
		}
		if opt.Dir != "" {
			o.Dir = opt.Dir
		}
	}
}
