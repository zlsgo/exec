package exec

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/sohaha/zlsgo/zfile"
	"github.com/sohaha/zlsgo/zshell"
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

func TestWrapOptionsUsesOverrideValues(t *testing.T) {
	engine := &Engine{
		options: &Options{
			Env: []string{"ENGINE=1"},
			Dir: "engine-dir",
		},
	}

	var shellOpt zshell.Options
	engine.wrapOptions(func(o *Options) {
		o.Env = []string{"REQUEST=2"}
		o.Dir = "request-dir"
	})(&shellOpt)

	if got, want := shellOpt.Dir, "request-dir"; got != want {
		t.Fatalf("wrapped dir = %q, want %q", got, want)
	}

	if !reflect.DeepEqual(shellOpt.Env, []string{"REQUEST=2"}) {
		t.Fatalf("wrapped env = %#v, want override env", shellOpt.Env)
	}
}

func TestJoinCommandQuotesArgumentsWithSpaces(t *testing.T) {
	command := joinCommand([]string{`C:\Program Files\tool.cmd`, "hello world", "--quiet"})
	want := `"C:\\Program Files\\tool.cmd" "hello world" --quiet`
	if command != want {
		t.Fatalf("joinCommand() = %q, want %q", command, want)
	}
}

func TestKeepRunReleasesMergedContextAfterProcessExit(t *testing.T) {
	original := callbackRunCommand
	defer func() {
		callbackRunCommand = original
	}()

	done := make(chan int, 1)
	ctxReleased := make(chan struct{}, 1)
	callbackRunCommand = func(ctx context.Context, command string, callback func(str string, isStdout bool), opt func(o *zshell.Options)) (<-chan int, func(string), error) {
		go func() {
			<-ctx.Done()
			ctxReleased <- struct{}{}
		}()
		done <- 0
		close(done)
		return done, func(string) {}, nil
	}

	engine := &Engine{
		ctx:     context.Background(),
		options: &Options{},
		binFile: buildNoopBinaryPath(t),
	}

	codeCh, _, err := engine.KeepRun(context.Background(), nil, func(string, bool) {})
	if err != nil {
		t.Fatalf("KeepRun() error = %v", err)
	}

	if code := <-codeCh; code != 0 {
		t.Fatalf("KeepRun() code = %d, want 0", code)
	}

	select {
	case <-ctxReleased:
	case <-time.After(2 * time.Second):
		t.Fatal("merged KeepRun context was not released after process exit")
	}
}

func buildNoopBinaryPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "noop")
	if runtime.GOOS == "windows" {
		path += ".cmd"
	} else {
		path += ".sh"
	}

	if err := os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return path
}
