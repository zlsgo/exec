package exec

import (
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewUsesExistingBinFile(t *testing.T) {
	binFile := newTestScript(t)

	engine, err := New(embed.FS{}, func(options *Options) {
		options.BinFile = binFile
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Release()
	})

	if got, want := engine.Dir(), filepath.Dir(binFile); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}

	if got, want := filepath.Clean(engine.binFile), filepath.Clean(binFile); got != want {
		t.Fatalf("binFile = %q, want %q", got, want)
	}

	if got, want := filepath.Clean(engine.BinFile()), filepath.Clean(binFile); got != want {
		t.Fatalf("BinFile() = %q, want %q", got, want)
	}
}

func TestNewRejectsMissingBinarySource(t *testing.T) {
	_, err := New(embed.FS{}, func(options *Options) {
		options.BinFile = "missing-binary"
	})
	if err == nil {
		t.Fatal("New() expected error when no embedded file and no download URL")
	}

	if !strings.Contains(err.Error(), "download url is empty") {
		t.Fatalf("New() error = %q, want download url is empty", err)
	}
}

func TestCommandSkipsEmptyArgs(t *testing.T) {
	binFile := newTestScript(t)
	engine := &Engine{binFile: binFile}

	command, err := engine.command([]string{"--version", "", "--quiet"})
	if err != nil {
		t.Fatalf("command() error = %v", err)
	}

	if strings.Contains(command, "  ") {
		t.Fatalf("command() = %q, want filtered empty args", command)
	}

	if got, want := command, binFile+" --version --quiet"; got != want {
		t.Fatalf("command() = %q, want %q", got, want)
	}
}

func TestCommandRequiresExistingBinary(t *testing.T) {
	engine := &Engine{binFile: filepath.Join(t.TempDir(), "missing")}

	_, err := engine.command(nil)
	if err == nil {
		t.Fatal("command() expected error when binary does not exist")
	}
}

func TestReleaseKeepsExternalBinDirectory(t *testing.T) {
	binFile := newTestScript(t)
	dir := filepath.Dir(binFile)

	engine, err := New(embed.FS{}, func(options *Options) {
		options.BinFile = binFile
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := engine.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("external bin directory should remain after Release(): %v", err)
	}
}

func TestRunUsesQuotedPathAndOverrideOptions(t *testing.T) {
	binFile := buildTestBinary(t, filepath.Join(t.TempDir(), "bin space"))
	workDir := filepath.Join(t.TempDir(), "work space")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	engine, err := New(embed.FS{}, func(options *Options) {
		options.BinFile = binFile
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Release()
	})

	code, out, errOut, err := engine.Run(nil, []string{"value with spaces"}, func(options *Options) {
		options.Env = []string{"EXEC_TEST_TOKEN=override"}
		options.Dir = workDir
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if code != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", code, errOut)
	}

	normalizedOut := strings.ReplaceAll(strings.TrimSpace(out), "\\", "/")
	normalizedDir := strings.ReplaceAll(workDir, "\\", "/")
	// On macOS, /var/folders is a symlink to /private/var/folders,
	// so os.Getwd() returns the resolved path. We need to compare
	// the resolved path with the actual output.
	realDir, err := filepath.EvalSymlinks(workDir)
	if err == nil {
		normalizedDir = strings.ReplaceAll(realDir, "\", "/")
	}
	if !strings.Contains(normalizedOut, "override|"+normalizedDir+"|value with spaces") {
		t.Fatalf("Run() output = %q", out)
	}
}

func TestKeepRunStreamsOutput(t *testing.T) {
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

	var lines []string
	codeCh, _, err := engine.KeepRun(nil, []string{"stream"}, func(str string, isStdout bool) {
		channel := "stderr"
		if isStdout {
			channel = "stdout"
		}
		lines = append(lines, channel+":"+str)
	})
	if err != nil {
		t.Fatalf("KeepRun() error = %v", err)
	}

	code := <-codeCh
	if code != 0 {
		t.Fatalf("KeepRun() exit code = %d", code)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		joined := strings.Join(lines, "\n")
		if strings.Contains(joined, "stdout:fixture-stream") && strings.Contains(joined, "stderr:fixture-err") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "stdout:fixture-stream") {
		t.Fatalf("KeepRun() stdout lines = %q", joined)
	}
	if !strings.Contains(joined, "stderr:fixture-err") {
		t.Fatalf("KeepRun() stderr lines = %q", joined)
	}
}

func newTestScript(t *testing.T) string {
	t.Helper()

	return newTestScriptInDir(t, t.TempDir())
}

func newTestScriptInDir(t *testing.T, dir string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "test-bin.cmd")
		content := "@echo off\r\n" +
			"if \"%~1\"==\"stream\" (\r\n" +
			"  echo fixture-stream\r\n" +
			"  echo fixture-err 1>&2\r\n" +
			"  exit /b 0\r\n" +
			")\r\n" +
			"echo %EXEC_TEST_TOKEN%^|%CD%^|%~1\r\n"
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("write test script: %v", err)
		}
		return path
	}

	path := filepath.Join(dir, "test-bin.sh")
	content := "#!/bin/sh\n" +
		"if [ \"$1\" = \"stream\" ]; then\n" +
		"  printf 'fixture-stream\\n'\n" +
		"  printf 'fixture-err\\n' >&2\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf '%s|%s|%s\\n' \"$EXEC_TEST_TOKEN\" \"$PWD\" \"$1\"\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write test script: %v", err)
	}
	return path
}

func buildTestBinary(t *testing.T, dir string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	output := filepath.Join(dir, "fixture-bin")
	if runtime.GOOS == "windows" {
		output += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", output, "./testdata/fixtureexec")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build test binary: %v\n%s", err, out)
	}

	return output
}
