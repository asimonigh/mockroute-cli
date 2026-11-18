package device

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

type fakeRunner struct {
	out    string
	err    error
	calls  []string
	argv   [][]string
	stdins [][]byte
}

func (f *fakeRunner) Run(_ context.Context, stdin io.Reader, args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	f.argv = append(f.argv, args)
	var in []byte
	if stdin != nil {
		in, _ = io.ReadAll(stdin)
	}
	f.stdins = append(f.stdins, in)
	return f.out, f.err
}

func code(err error) string {
	var ce *clierr.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestFindPackage(t *testing.T) {
	const base = "package:com.simonigh.fakemygpslocation"
	tests := []struct {
		name, out, explicit, want, code string
	}{
		{"none", "", "", "", "app_not_installed"},
		{"one", base + ".internal\r\n", "", "com.simonigh.fakemygpslocation.internal", ""},
		{"two", base + "\n" + base + ".debug\n", "", "", "multiple_packages"},
		{"explicit missing", base + "\n", "com.simonigh.fakemygpslocation.debug", "", "app_not_installed"},
		{"explicit present", base + "\n" + base + ".debug\n", "com.simonigh.fakemygpslocation.debug", "com.simonigh.fakemygpslocation.debug", ""},
		{"other ignored", base + ".other\n", "", "", "app_not_installed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRunner{out: tc.out}
			got, err := FindPackage(context.Background(), f, "S1", tc.explicit)
			if code(err) != tc.code {
				t.Fatalf("code = %q (%v), want %q", code(err), err, tc.code)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			if f.calls[0] != "-s S1 shell pm list packages com.simonigh.fakemygpslocation" {
				t.Fatalf("call = %q", f.calls[0])
			}
		})
	}
	f := &fakeRunner{out: base + "\n" + base + ".debug\n"}
	_, err := FindPackage(context.Background(), f, "S1", "")
	if !strings.Contains(err.Error(), "com.simonigh.fakemygpslocation.debug") {
		t.Errorf("message should list packages: %v", err)
	}
}

func TestFindPackageRunnerFailure(t *testing.T) {
	f := &fakeRunner{out: "error: device offline\n", err: errors.New("exit status 1")}
	_, err := FindPackage(context.Background(), f, "S1", "")
	if code(err) != "adb_failed" || !strings.Contains(err.Error(), "error: device offline") {
		t.Fatalf("got %q: %v", code(err), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f = &fakeRunner{err: errors.New("signal: killed")}
	_, err = FindPackage(ctx, f, "S1", "")
	if code(err) != "timeout" {
		t.Fatalf("got %q: %v", code(err), err)
	}
}
