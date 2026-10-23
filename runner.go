package exec

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
)

// runSpec holds the process settings resolved from Options.
type runSpec struct {
	Env        []string
	Dir        string
	CloseStdin bool
}

// apply copies the spec onto a command. A nil Env keeps the inherited
// environment; otherwise the entries are appended so the child keeps PATH
// while the override wins for duplicated keys.
func (s runSpec) apply(cmd *exec.Cmd) {
	if s.Dir != "" {
		cmd.Dir = s.Dir
	}
	if s.Env != nil {
		cmd.Env = append(os.Environ(), s.Env...)
	}
}

func newCommand(ctx context.Context, argv []string, spec runSpec) (*exec.Cmd, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, errors.New("no command")
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	spec.apply(cmd)
	return cmd, nil
}

func exitCode(cmd *exec.Cmd, runErr error) int {
	if cmd.ProcessState == nil {
		if runErr != nil {
			return -1
		}
		return 0
	}
	return cmd.ProcessState.ExitCode()
}

// runCommand executes argv directly, without any shell-style splitting or quoting.
func runCommand(ctx context.Context, argv []string, spec runSpec) (code int, outStr string, errStr string, err error) {
	cmd, err := newCommand(ctx, argv, spec)
	if err != nil {
		return -1, "", "", err
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	return exitCode(cmd, err), stdout.String(), stderr.String(), err
}

// callbackRunCommand executes argv directly and streams output line by line.
// The returned writer feeds the process stdin.
func callbackRunCommand(ctx context.Context, argv []string, callback func(str string, isStdout bool), spec runSpec) (<-chan int, func(string), error) {
	cmd, err := newCommand(ctx, argv, spec)
	if err != nil {
		return nil, nil, err
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	var (
		stdinMu sync.Mutex
		cbMu    sync.Mutex
	)
	stdinClosed := false

	input := func(s string) {
		stdinMu.Lock()
		defer stdinMu.Unlock()
		if stdinClosed {
			return
		}
		_, _ = io.WriteString(stdin, s)
	}

	if spec.CloseStdin {
		stdinMu.Lock()
		stdinClosed = true
		_ = stdin.Close()
		stdinMu.Unlock()
	}

	emit := func(str string, isStdout bool) {
		// The two pumps run concurrently; serialise user callbacks so callers
		// can append to shared state without extra locking.
		cbMu.Lock()
		defer cbMu.Unlock()
		callback(str, isStdout)
	}

	scan := func(r io.Reader, isStdout bool) {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			emit(scanner.Text(), isStdout)
		}
	}

	codeCh := make(chan int, 1)
	go func() {
		var pumps sync.WaitGroup
		pumps.Add(2)
		go func() {
			defer pumps.Done()
			scan(stdout, true)
		}()
		go func() {
			defer pumps.Done()
			scan(stderr, false)
		}()

		// Wait for both pipes to drain before reaping the process, as
		// required by os/exec's StdoutPipe contract.
		pumps.Wait()

		waitErr := cmd.Wait()

		stdinMu.Lock()
		stdinClosed = true
		_ = stdin.Close()
		stdinMu.Unlock()

		codeCh <- exitCode(cmd, waitErr)
		close(codeCh)
	}()

	return codeCh, input, nil
}
