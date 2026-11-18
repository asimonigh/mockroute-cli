// Package adb locates and drives the adb binary.
package adb

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

// Runner calls adb.
type Runner interface {
	// Run calls adb with args and returns stdout and stderr together. stdin may be nil.
	// A non-zero exit returns the output and an error.
	Run(ctx context.Context, stdin io.Reader, args ...string) (string, error)
}

// ExecRunner runs the adb binary at Path.
type ExecRunner struct{ Path string }

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, stdin io.Reader, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.Path, args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
