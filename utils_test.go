package exec

import (
	"context"
	"embed"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sohaha/zlsgo/zfile"
)

func TestInitTmpDirReturnsExistingDirectory(t *testing.T) {
	path := initTmpDir()
	if path == "" {
		t.Fatal("initTmpDir() returned empty path")
	}

	if !zfile.DirExist(path) {
		t.Fatalf("initTmpDir() = %q, directory does not exist", path)
	}
}

func TestWriteFileWritesBinaryToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool.bin")
	data := []byte("fixture")

	if err := writeFile(path, data); err != nil {
		t.Fatalf("writeFile() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(got) != string(data) {
		t.Fatalf("file content = %q, want %q", got, data)
	}
}

func TestWriteFileRejectsEmptyBinary(t *testing.T) {
	err := writeFile(filepath.Join(t.TempDir(), "tool.bin"), nil)
	if err == nil {
		t.Fatal("writeFile() expected error for empty binary")
	}
}

func TestMergeNilContextUsesEngineContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	engine := &Engine{ctx: ctx}

	runCtx, stop := merge(engine, nil)
	defer stop()

	if runCtx != ctx {
		t.Fatal("merge() with nil ctx should reuse engine context")
	}

	cancel()
	select {
	case <-runCtx.Done():
	default:
		t.Fatal("engine context should be cancelled")
	}
}

func TestMergeCancelsWhenEngineContextEnds(t *testing.T) {
	engineCtx, engineCancel := context.WithCancel(context.Background())
	userCtx, userCancel := context.WithCancel(context.Background())
	defer userCancel()

	engine := &Engine{ctx: engineCtx}
	runCtx, stop := merge(engine, userCtx)
	defer stop()

	engineCancel()

	select {
	case <-runCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("merged context was not cancelled in time")
	}

	if err := runCtx.Err(); err == nil {
		t.Fatal("merged context should be cancelled after engine context ends")
	}
}

func TestRunSpecUsesOverrideValues(t *testing.T) {
	engine := &Engine{
		options: &Options{
			Env:        []string{"ENGINE=1"},
			Dir:        "engine-dir",
			CloseStdin: true,
		},
	}

	spec := engine.runSpec(func(o *Options) {
		o.Env = []string{"REQUEST=2"}
		o.Dir = "request-dir"
		o.CloseStdin = false
	})

	if got, want := spec.Dir, "request-dir"; got != want {
		t.Fatalf("spec dir = %q, want %q", got, want)
	}

	if !reflect.DeepEqual(spec.Env, []string{"REQUEST=2"}) {
		t.Fatalf("spec env = %#v, want override env", spec.Env)
	}

	if spec.CloseStdin {
		t.Fatal("spec CloseStdin = true, want override false")
	}
}

func TestArgvKeepsArgumentBoundaries(t *testing.T) {
	binFile := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(binFile, []byte("fixture"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	engine := &Engine{binFile: binFile}
	args := []string{`C:\Program Files\tool.cmd`, "hello world", `--sql=SELECT E'\n'`, "--quiet"}
	argv, err := engine.argv(args)
	if err != nil {
		t.Fatalf("argv() error = %v", err)
	}

	want := append([]string{binFile}, args...)
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv() = %#v, want %#v", argv, want)
	}
}

func TestKeepRunStopsOnContextCancel(t *testing.T) {
	binFile := buildTestBinary(t, t.TempDir())
	engine, err := New(embed.FS{}, func(options *Options) {
		options.BinFile = binFile
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Release()
	})

	ctx, cancel := context.WithCancel(context.Background())
	codeCh, _, err := engine.KeepRun(ctx, []string{"sleep"}, func(string, bool) {})
	if err != nil {
		t.Fatalf("KeepRun() error = %v", err)
	}
	cancel()

	select {
	case code := <-codeCh:
		if code == 0 {
			t.Fatalf("KeepRun() code = %d, want non-zero after cancel", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("KeepRun() did not stop after context cancel")
	}
}
