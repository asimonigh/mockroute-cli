package device

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

const testPkg = "com.x"

func newClient(f *fakeRunner) *Client {
	return &Client{Runner: f, Serial: "S1", Package: testPkg}
}

func bundle(json string) string {
	return "* daemon not running; starting now at tcp:5037\r\n* daemon started successfully\r\n" +
		"Result: Bundle[{result=" + base64.StdEncoding.EncodeToString([]byte(json)) + "}]\r\n"
}

func asErr(t *testing.T, err error) *clierr.Error {
	t.Helper()
	var ce *clierr.Error
	if !errors.As(err, &ce) {
		t.Fatalf("want *clierr.Error, got %v", err)
	}
	return ce
}

func TestCallDecodesResult(t *testing.T) {
	f := &fakeRunner{out: bundle(`{"protocol":1}`)}
	got, err := newClient(f).Call(context.Background(), "version", "")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"protocol":1}` {
		t.Fatalf("got %s", got)
	}
}

func TestCallMapsAppErrors(t *testing.T) {
	f := &fakeRunner{out: bundle(`{"error":{"code":"pro_required","message":"Pro needed","target":"fused"}}`)}
	_, err := newClient(f).Call(context.Background(), "start", "")
	ce := asErr(t, err)
	if ce.Code != "pro_required" || ce.Message != "Pro needed" || ce.ExitCode() != 4 {
		t.Fatalf("got %+v exit %d", ce, ce.ExitCode())
	}
	if ce.Details["target"] != "fused" {
		t.Fatalf("details %v", ce.Details)
	}
	if _, ok := ce.Details["missing"]; ok {
		t.Fatalf("missing should be absent: %v", ce.Details)
	}

	f = &fakeRunner{out: bundle(`{"error":{"code":"setup_incomplete","message":"m","missing":["mock_app"]}}`)}
	_, err = newClient(f).Call(context.Background(), "start", "")
	ce = asErr(t, err)
	if ce.Code != "setup_incomplete" || !reflect.DeepEqual(ce.Details["missing"], []any{"mock_app"}) {
		t.Fatalf("got %+v", ce)
	}
	if _, ok := ce.Details["target"]; ok {
		t.Fatalf("target should be absent: %v", ce.Details)
	}
}

func TestCallWithoutProvider(t *testing.T) {
	f := &fakeRunner{out: "Error while accessing provider:com.x.cli\njava.lang.IllegalArgumentException: Unknown authority com.x.cli\n"}
	_, err := newClient(f).Call(context.Background(), "version", "")
	ce := asErr(t, err)
	if ce.Code != "app_outdated" || ce.Message != "This MockRoute version has no CLI support. Update MockRoute." {
		t.Fatalf("got %+v", ce)
	}
}

func TestCallGarbage(t *testing.T) {
	f := &fakeRunner{out: "  something odd \n"}
	_, err := newClient(f).Call(context.Background(), "version", "")
	ce := asErr(t, err)
	if ce.Code != "protocol_error" || !strings.Contains(ce.Message, "something odd") {
		t.Fatalf("got %+v", ce)
	}
}

func TestCallRunnerFailure(t *testing.T) {
	f := &fakeRunner{out: "adb: device offline", err: errors.New("exit status 1")}
	_, err := newClient(f).Call(context.Background(), "version", "")
	if code(err) != "adb_failed" {
		t.Fatalf("got %v", err)
	}
}

func TestCallArg(t *testing.T) {
	f := &fakeRunner{out: bundle(`{}`)}
	c := newClient(f)
	if _, err := c.Call(context.Background(), "speed", "40"); err != nil {
		t.Fatal(err)
	}
	want := []string{"-s", "S1", "shell", "content", "call", "--uri", "content://com.x.cli", "--method", "speed", "--arg", "40"}
	if !reflect.DeepEqual(f.argv[0], want) {
		t.Fatalf("got %v", f.argv[0])
	}
	if _, err := c.Call(context.Background(), "version", ""); err != nil {
		t.Fatal(err)
	}
	want = []string{"-s", "S1", "shell", "content", "call", "--uri", "content://com.x.cli", "--method", "version"}
	if !reflect.DeepEqual(f.argv[1], want) {
		t.Fatalf("got %v", f.argv[1])
	}
}

func TestVersionProtocol(t *testing.T) {
	f := &fakeRunner{out: bundle(`{"protocol":1,"app_version":"1.4.0","build_type":"debug","package":"com.x"}`)}
	v, err := newClient(f).Version(context.Background())
	if err != nil || v.AppVersion != "1.4.0" || v.Protocol != 1 {
		t.Fatalf("got %+v %v", v, err)
	}
	f = &fakeRunner{out: bundle(`{"protocol":2}`)}
	if _, err := newClient(f).Version(context.Background()); code(err) != "cli_outdated" {
		t.Fatalf("got %v", err)
	}
	f = &fakeRunner{out: bundle(`{"protocol":0}`)}
	if _, err := newClient(f).Version(context.Background()); code(err) != "app_outdated" {
		t.Fatalf("got %v", err)
	}
}

func TestSDKInt(t *testing.T) {
	f := &fakeRunner{out: "34\r\n"}
	n, err := newClient(f).SDKInt(context.Background())
	if err != nil || n != 34 {
		t.Fatalf("got %d %v", n, err)
	}
	if f.calls[0] != "-s S1 shell getprop ro.build.version.sdk" {
		t.Fatalf("call %q", f.calls[0])
	}
	f = &fakeRunner{out: "nope"}
	if _, err := newClient(f).SDKInt(context.Background()); code(err) != "protocol_error" {
		t.Fatalf("got %v", err)
	}
}

func writeGPX(t *testing.T) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trajet été.gpx")
	data := []byte("<gpx>é</gpx>")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

func TestSendGPXContentWrite(t *testing.T) {
	path, data := writeGPX(t)
	f := &fakeRunner{out: ""}
	method, err := newClient(f).SendGPX(context.Background(), path, 28)
	if err != nil || method != "load" {
		t.Fatalf("got %q %v", method, err)
	}
	want := []string{"-s", "S1", "shell", "content", "write", "--uri", "content://com.x.cli/gpx"}
	if !reflect.DeepEqual(f.argv[0], want) {
		t.Fatalf("got %v", f.argv[0])
	}
	if !bytes.Equal(f.stdins[0], data) {
		t.Fatalf("stdin %q", f.stdins[0])
	}
}

func TestSendGPXPush(t *testing.T) {
	path, _ := writeGPX(t)
	f := &fakeRunner{out: "1 file pushed. 0.1 MB/s\n"}
	method, err := newClient(f).SendGPX(context.Background(), path, 27)
	if err != nil || method != "load_pushed" {
		t.Fatalf("got %q %v", method, err)
	}
	want := []string{"-s", "S1", "push", path, "/sdcard/Android/data/com.x/files/cli/route.gpx"}
	if !reflect.DeepEqual(f.argv[0], want) {
		t.Fatalf("got %v", f.argv[0])
	}
}

func TestSendGPXErrors(t *testing.T) {
	_, err := newClient(&fakeRunner{}).SendGPX(context.Background(), filepath.Join(t.TempDir(), "absent.gpx"), 30)
	if code(err) != "file_not_found" {
		t.Fatalf("got %v", err)
	}
	path, _ := writeGPX(t)
	f := &fakeRunner{out: "Exception occurred while executing 'write':\njava.lang.SecurityException\n"}
	if _, err := newClient(f).SendGPX(context.Background(), path, 30); code(err) != "protocol_error" {
		t.Fatalf("got %v", err)
	}
	f = &fakeRunner{out: "adb: failed to copy", err: errors.New("exit status 1")}
	if _, err := newClient(f).SendGPX(context.Background(), path, 27); code(err) != "adb_failed" {
		t.Fatalf("got %v", err)
	}
}

func TestStart(t *testing.T) {
	f := &fakeRunner{out: "Starting: Intent { cmp=com.x/... }\nStatus: ok\n"}
	if err := newClient(f).Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"-s", "S1", "shell", "am", "start", "-n", "com.x/com.simonigh.fakemygpslocation.cli.CliStartActivity"}
	if !reflect.DeepEqual(f.argv[0], want) {
		t.Fatalf("got %v", f.argv[0])
	}
	f = &fakeRunner{out: "Error: Activity not started, unknown error code 1\n"}
	if code(newClient(f).Start(context.Background())) != "start_failed" {
		t.Fatal("want start_failed")
	}
	f = &fakeRunner{out: "closed", err: errors.New("exit status 1")}
	if code(newClient(f).Start(context.Background())) != "adb_failed" {
		t.Fatal("want adb_failed")
	}
}

func TestSendGPXPushIgnoresPathWords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ErrorLog.gpx")
	if err := os.WriteFile(path, []byte("<gpx/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{out: path + ": 1 file pushed. 0.1 MB/s\n"}
	method, err := newClient(f).SendGPX(context.Background(), path, 27)
	if err != nil || method != "load_pushed" {
		t.Fatalf("got %q %v", method, err)
	}
}

func TestSendGPXContentWriteProviderMissing(t *testing.T) {
	path, _ := writeGPX(t)
	f := &fakeRunner{out: "Error while accessing provider:com.x.cli\nUnknown authority com.x.cli\n"}
	if _, err := newClient(f).SendGPX(context.Background(), path, 30); code(err) != "app_outdated" {
		t.Fatalf("got %v", err)
	}
}

func TestStartException(t *testing.T) {
	f := &fakeRunner{out: "Exception occurred while executing 'start':\n"}
	if code(newClient(f).Start(context.Background())) != "start_failed" {
		t.Fatal("want start_failed")
	}
}

type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, _ io.Reader, _ ...string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestStartTimesOut(t *testing.T) {
	old := startTimeout
	startTimeout = 10 * time.Millisecond
	defer func() { startTimeout = old }()
	c := &Client{Runner: blockingRunner{}, Serial: "S1", Package: testPkg}
	if got := code(c.Start(context.Background())); got != "timeout" {
		t.Fatalf("got %q", got)
	}
}

func TestSendGPXContentWriteUnsupportedFallsBackToPush(t *testing.T) {
	path, _ := writeGPX(t)
	f := &fakeRunner{out: "[ERROR] Unsupported operation: write\nusage: adb shell content [subcommand]\n"}
	method, err := newClient(f).SendGPX(context.Background(), path, 30)
	if err != nil || method != "load_pushed" {
		t.Fatalf("got %q %v", method, err)
	}
	if len(f.argv) != 2 || f.argv[1][2] != "push" {
		t.Fatalf("argv %v", f.argv)
	}
}
