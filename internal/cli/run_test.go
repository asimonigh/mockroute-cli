package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/asimonigh/mockroute-cli/internal/adb"
)

const (
	pkg     = "com.simonigh.fakemygpslocation"
	devList = "List of devices attached\nS1\tdevice usb:1\n\n"
	pmList  = "package:" + pkg + "\n"
)

// resp is a scripted adb answer.
type resp struct {
	out string
	err error
}

// fakeRunner answers by joined argv and records every call.
type fakeRunner struct {
	responses map[string]resp
	calls     []string
	onCall    func(key string) // optional hook run after each call is recorded
	// sequences scripts successive answers of one key; the last answer repeats.
	sequences map[string][]resp
}

func (f *fakeRunner) Run(_ context.Context, _ io.Reader, args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	defer func() {
		if f.onCall != nil {
			f.onCall(key)
		}
	}()
	if seq := f.sequences[key]; len(seq) > 0 {
		r := seq[0]
		if len(seq) > 1 {
			f.sequences[key] = seq[1:]
		}
		return r.out, r.err
	}
	if r, ok := f.responses[key]; ok {
		return r.out, r.err
	}
	return "", errors.New("unscripted call: " + key)
}

func (f *fakeRunner) called(prefix string) bool {
	for _, c := range f.calls {
		if strings.Contains(c, prefix) {
			return true
		}
	}
	return false
}

func bundle(json string) string {
	return "Result: Bundle[{result=" + base64.StdEncoding.EncodeToString([]byte(json)) + "}]\n"
}

func provider(method string) string {
	return "-s S1 shell content call --uri content://" + pkg + ".cli --method " + method
}

const versionJSON = `{"protocol":1,"app_version":"1.4.0","build_type":"release","package":"` + pkg + `"}`

// baseResponses scripts a healthy device with the app answering version.
func baseResponses() map[string]resp {
	return map[string]resp{
		"devices -l":                          {out: devList},
		"-s S1 shell pm list packages " + pkg: {out: pmList},
		provider("version"):                   {out: bundle(versionJSON)},
	}
}

func run(t *testing.T, f *fakeRunner, args ...string) (int, string, string) {
	t.Helper()
	return runEnv(t, f, nil, args...)
}

// runEnv is run with a hook to adjust the Env (fake clock, for instance).
func runEnv(t *testing.T, f *fakeRunner, tweak func(*Env), args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdout: &stdout, Stderr: &stderr,
		Locator: adb.Locator{
			Getenv:   func(string) string { return "" },
			GOOS:     "linux",
			Exists:   func(string) bool { return false },
			LookPath: func(string) (string, error) { return "/bin/adb", nil },
		},
		NewRunner: func(string) adb.Runner { return f },
		Sleep:     func(time.Duration) {},
		Now:       time.Now,
		Version:   "1.2.3",
	}
	if tweak != nil {
		tweak(&env)
	}
	code := Run(args, env)
	return code, stdout.String(), stderr.String()
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	if strings.Count(s, "\n") != 1 || !strings.HasSuffix(s, "\n") {
		t.Fatalf("want exactly one line, got %q", s)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", s, err)
	}
	return m
}

func errCode(t *testing.T, out string) string {
	t.Helper()
	e, _ := decode(t, out)["error"].(map[string]any)
	if e == nil {
		t.Fatalf("no error in %q", out)
	}
	return e["code"].(string)
}

func TestStatusPassThrough(t *testing.T) {
	const status = `{"state":"idle","setup":{"ready":true,"missing":[]}}`
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("status")] = resp{out: bundle(status)}
	code, out, _ := run(t, f, "status")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !reflect.DeepEqual(decode(t, out), mustMap(t, status)) {
		t.Fatalf("got %s", out)
	}
}

func TestStopPassThrough(t *testing.T) {
	const stopped = `{"stopped":true}`
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("stop")] = resp{out: bundle(stopped)}
	code, out, _ := run(t, f, "stop")
	if code != 0 || !reflect.DeepEqual(decode(t, out), mustMap(t, stopped)) {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func mustMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestVersionWithoutDevice(t *testing.T) {
	f := &fakeRunner{responses: map[string]resp{"devices -l": {out: "List of devices attached\n\n"}}}
	code, out, _ := run(t, f, "version")
	if code != 0 || out != `{"cli":"1.2.3"}`+"\n" {
		t.Fatalf("exit %d: %q", code, out)
	}
}

func TestVersionWithApp(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	code, out, _ := run(t, f, "version")
	m := decode(t, out)
	app, _ := m["app"].(map[string]any)
	if code != 0 || m["cli"] != "1.2.3" || app["app_version"] != "1.4.0" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestVersionNewerAppStillShowsApp(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("version")] = resp{out: bundle(`{"protocol":2,"app_version":"2.0.0","build_type":"release","package":"` + pkg + `"}`)}
	code, out, _ := run(t, f, "version")
	m := decode(t, out)
	app, _ := m["app"].(map[string]any)
	if code != 0 || app == nil || app["protocol"] != float64(2) {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestVersionOutdatedAppOmitsApp(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("version")] = resp{out: bundle(`{"protocol":0,"app_version":"0.1","build_type":"release","package":"` + pkg + `"}`)}
	code, out, _ := run(t, f, "version")
	if code != 0 || out != `{"cli":"1.2.3"}`+"\n" {
		t.Fatalf("exit %d: %q", code, out)
	}
}

func TestVersionCallsAppOnce(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	if code, out, _ := run(t, f, "version"); code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	n := 0
	for _, c := range f.calls {
		if c == provider("version") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("version called %d times: %v", n, f.calls)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, out, _ := run(t, &fakeRunner{}, "frobnicate")
	if code != 1 || errCode(t, out) != "usage" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestNoCommandPrintsUsageOnStderr(t *testing.T) {
	code, out, errOut := run(t, &fakeRunner{})
	if code != 1 || out != "" || !strings.Contains(errOut, "Usage") {
		t.Fatalf("exit %d out %q err %q", code, out, errOut)
	}
}

func TestHelp(t *testing.T) {
	code, out, errOut := run(t, &fakeRunner{}, "status", "--help")
	if code != 1 || out != "" || !strings.Contains(errOut, "Usage") {
		t.Fatalf("exit %d out %q err %q", code, out, errOut)
	}
}

func TestGlobalFlagsAnywhere(t *testing.T) {
	for _, args := range [][]string{
		{"status", "--serial", "X"},
		{"--serial", "X", "status"},
		{"status", "--serial=X"},
		{"--package", pkg, "status", "--serial", "X"},
	} {
		f := &fakeRunner{responses: map[string]resp{
			"devices -l":                                       {out: "X\tdevice\n"},
			"-s X shell pm list packages " + pkg:               {out: pmList},
			strings.ReplaceAll(provider("version"), "S1", "X"): {out: bundle(versionJSON)},
			strings.ReplaceAll(provider("status"), "S1", "X"):  {out: bundle(`{"state":"idle"}`)},
		}}
		code, out, _ := run(t, f, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, out)
		}
		for _, c := range f.calls[1:] {
			if !strings.HasPrefix(c, "-s X ") {
				t.Fatalf("%v: call without -s X: %s", args, c)
			}
		}
	}
}

func TestAppOutdated(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("version")] = resp{
		out: "Error while accessing provider:" + pkg + ".cli\njava.lang.IllegalArgumentException: Unknown authority " + pkg + ".cli\n",
		err: errors.New("exit status 1"),
	}
	code, out, _ := run(t, f, "status")
	if code != 3 || errCode(t, out) != "app_outdated" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestAdbDevicesFailure(t *testing.T) {
	f := &fakeRunner{responses: map[string]resp{"devices -l": {out: "boom", err: errors.New("exit status 1")}}}
	code, out, _ := run(t, f, "status")
	if code != 2 || errCode(t, out) != "adb_failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestReorderValueFlags(t *testing.T) {
	fs := newFlagSet(&globalFlags{})
	got, err := reorder(fs, []string{"a", "--serial", "X", "b", "--package=P", "--", "--adb"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--serial", "X", "--package=P", "a", "b", "--adb"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestReorderNegativeNumbers(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"point", "-33.86", "151.2", "--serial", "S"}, []string{"--serial", "S", "point", "-33.86", "151.2"}},
		{[]string{"point", "43.6", "-1.444"}, []string{"point", "43.6", "-1.444"}},
		{[]string{"point", "43.6", "-1.444", "--speed", "-5"}, []string{"--speed", "-5", "point", "43.6", "-1.444"}},
		{[]string{"point", "--accuracy", "25", "-33.86", "151.2"}, []string{"--accuracy", "25", "point", "-33.86", "151.2"}},
		{[]string{"point", "-33.86,151.2"}, []string{"point", "-33.86,151.2"}},
		{[]string{"point", "-33.86;151.2"}, []string{"point", "-33.86;151.2"}},
		{[]string{"point", "-33,86", "151,2"}, []string{"point", "-33,86", "151,2"}},
		{[]string{"point", "43,6045", "-1,444"}, []string{"point", "43,6045", "-1,444"}},
		{[]string{"point", "-.5,2"}, []string{"point", "-.5,2"}},
	}
	for _, c := range cases {
		got, err := reorder(newFlagSet(&globalFlags{}), c.args)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %v want %v", c.args, got, c.want)
		}
	}
}
