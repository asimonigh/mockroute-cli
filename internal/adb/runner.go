// Package adb locates and drives the adb binary.
package adb

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
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

// RunError turns a failed Runner call into a typed error: timeout when the
// context is done, adb_failed otherwise, with the adb output in the message.
func RunError(ctx context.Context, out string, err error) *clierr.Error {
	if ctx.Err() != nil {
		return clierr.New("timeout", "adb call interrupted: %v", ctx.Err())
	}
	if msg := strings.TrimSpace(out); msg != "" {
		return clierr.New("adb_failed", "adb failed: %s (%v)", msg, err)
	}
	return clierr.New("adb_failed", "adb failed: %v", err)
}
