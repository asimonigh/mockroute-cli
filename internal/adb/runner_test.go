package adb

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestExecRunner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	r := ExecRunner{Path: "/bin/sh"}
	out, err := r.Run(context.Background(), nil, "-c", "echo out; echo err >&2")
	if err != nil || !strings.Contains(out, "out") || !strings.Contains(out, "err") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	out, err = r.Run(context.Background(), strings.NewReader("piped"), "-c", "cat; exit 3")
	if err == nil || out != "piped" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
