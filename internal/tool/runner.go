package tool

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Command is one subprocess invocation. Name, Args, and OnLine are used by
// every production caller. Dir and Env are a deliberate test seam: no
// production path sets them today, but they let runner tests exercise the
// working-directory and environment handling in Run without stubbing the
// runner itself.
type Command struct {
	Name   string
	Args   []string
	Dir    string
	Env    []string
	OnLine func(string)
}

type Runner interface {
	Run(ctx context.Context, cmd Command) (string, error)
}

type DefaultRunner struct{}

func (DefaultRunner) Run(ctx context.Context, c Command) (string, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	if len(c.Env) > 0 {
		cmd.Env = append(cmd.Environ(), c.Env...)
	}

	if c.OnLine == nil {
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		err := cmd.Run()
		return buf.String(), err
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	// Drain both pipes concurrently: reading one stream to EOF before the
	// other can deadlock when the child fills the unread pipe's kernel
	// buffer while writing. Lines are delivered to OnLine serially in
	// arrival order through the channel, so callers observe one line at a
	// time exactly as before.
	lines := make(chan streamResult)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		drainStream(stdoutPipe, "stdout", lines)
	}()
	go func() {
		defer wg.Done()
		drainStream(stderrPipe, "stderr", lines)
	}()
	go func() {
		wg.Wait()
		close(lines)
	}()

	var fullOutput bytes.Buffer
	var streamErr error
	for res := range lines {
		if res.err != nil {
			if streamErr == nil {
				streamErr = res.err
			}
			continue
		}
		fullOutput.WriteString(res.line)
		fullOutput.WriteByte('\n')
		c.OnLine(res.line)
	}

	waitErr := cmd.Wait()
	// The process exit status is authoritative for the outcome: when the
	// install itself failed, its error is what callers act on, with any
	// stream failure attached as context. A stream-only failure still
	// fails the run, since partial output cannot attest a success.
	if waitErr != nil {
		if streamErr != nil {
			return fullOutput.String(), fmt.Errorf("%w (stream: %v)", waitErr, streamErr)
		}
		return fullOutput.String(), waitErr
	}
	if streamErr != nil {
		return fullOutput.String(), streamErr
	}
	return fullOutput.String(), nil
}

// streamResult carries one scanned line or a terminal stream error.
type streamResult struct {
	line string
	err  error
}

// maxStreamLine bounds a single scanned line. Lines beyond the bufio default
// are preserved up to this limit; anything larger surfaces as a stream error
// instead of being silently truncated.
const maxStreamLine = 4 * 1024 * 1024

// drainStream forwards every line from one pipe to ch, then reports a stream
// read failure if scanning did not end cleanly. After a scan error the
// remainder is discarded up to EOF so a voluminous child can still terminate
// instead of blocking forever on a pipe nobody drains.
func drainStream(r io.Reader, stream string, ch chan<- streamResult) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxStreamLine)
	for scanner.Scan() {
		ch <- streamResult{line: scanner.Text()}
	}
	if err := scanner.Err(); err != nil {
		_, _ = io.Copy(io.Discard, r)
		ch <- streamResult{err: fmt.Errorf("%s: %w", stream, err)}
	}
}
