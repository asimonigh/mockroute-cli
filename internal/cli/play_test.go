package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	loadOK = `{"loaded":true,"route":{},"pace_source":"gpx_times"}`
	ready  = `"setup":{"ready":true,"missing":[]}`
)

func stateJSON(state string) string {
	return `{"state":"` + state + `",` + ready + `}`
}

// playRunner scripts a device ready to play on the given Android level.
func playRunner(sdk string, states ...string) *fakeRunner {
	f := &fakeRunner{responses: baseResponses(), sequences: map[string][]resp{}}
	// The first status read is the setup check, ready and idle.
	seq := []resp{{out: bundle(stateJSON("idle"))}}
	for _, s := range states {
		seq = append(seq, resp{out: bundle(stateJSON(s))})
	}
	f.sequences[provider("status")] = seq
	f.responses["-s S1 shell getprop ro.build.version.sdk"] = resp{out: sdk + "\n"}
	f.responses["-s S1 shell content write --uri content://"+pkg+".cli/gpx"] = resp{}
	f.responses["-s S1 push TRACK /sdcard/Android/data/"+pkg+"/files/cli/route.gpx"] = resp{}
	f.responses[provider("load")] = resp{out: bundle(loadOK)}
	f.responses[provider("load_pushed")] = resp{out: bundle(loadOK)}
	f.responses["-s S1 shell am start -n "+pkg+"/com.simonigh.fakemygpslocation.cli.CliStartActivity"] = resp{out: "Status: ok\n"}
	return f
}

func gpxFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "trajet.gpx")
	if err := os.WriteFile(p, []byte("<gpx/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeClock makes Sleep advance Now.
func fakeClock(env *Env) {
	now := time.Unix(1000, 0)
	env.Now = func() time.Time { return now }
	env.Sleep = func(d time.Duration) { now = now.Add(d) }
}

// withTrack points the scripted push answer at the real temp file.
func withTrack(f *fakeRunner, path string) {
	k := "-s S1 push TRACK /sdcard/Android/data/" + pkg + "/files/cli/route.gpx"
	f.responses["-s S1 push "+path+" /sdcard/Android/data/"+pkg+"/files/cli/route.gpx"] = f.responses[k]
}

func TestPlayHappyPath(t *testing.T) {
	f := playRunner("34", "simulating")
	code, out, _ := runEnv(t, f, fakeClock, "play", gpxFile(t))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	want := []string{
		"devices -l",
		"-s S1 shell pm list packages " + pkg,
		provider("version"),
		provider("status"),
		"-s S1 shell getprop ro.build.version.sdk",
		"-s S1 shell content write --uri content://" + pkg + ".cli/gpx",
		provider("load"),
		"-s S1 shell am start -n " + pkg + "/com.simonigh.fakemygpslocation.cli.CliStartActivity",
		provider("status"),
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls:\n%s", strings.Join(f.calls, "\n"))
	}
	m := decode(t, out)
	if m["state"] != "simulating" || m["pace_source"] != "gpx_times" {
		t.Fatalf("got %s", out)
	}
}

func TestPlaySpeedArg(t *testing.T) {
	f := playRunner("34", "simulating")
	f.responses[provider("load")+" --arg 40"] = resp{out: bundle(loadOK)}
	if code, out, _ := runEnv(t, f, fakeClock, "play", "--speed", "40", gpxFile(t)); code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !f.called("--method load --arg 40") {
		t.Fatalf("calls: %v", f.calls)
	}
	g := playRunner("34", "simulating")
	runEnv(t, g, fakeClock, "play", gpxFile(t))
	if g.called("--arg") {
		t.Fatalf("unexpected --arg: %v", g.calls)
	}
}

func TestPlayInvalidSpeed(t *testing.T) {
	for _, s := range []string{"0.5", "0", "301"} {
		f := playRunner("34")
		code, out, _ := run(t, f, "play", "--speed", s, gpxFile(t))
		if code != 6 || errCode(t, out) != "invalid_speed" || len(f.calls) != 0 {
			t.Fatalf("speed %s: exit %d %s calls %v", s, code, out, f.calls)
		}
	}
}

func TestPlayMissingFile(t *testing.T) {
	f := playRunner("34")
	code, out, _ := run(t, f, "play", filepath.Join(t.TempDir(), "nope.gpx"))
	if code != 1 || errCode(t, out) != "file_not_found" || len(f.calls) != 0 {
		t.Fatalf("exit %d: %s calls %v", code, out, f.calls)
	}
}

func TestPlayDirectoryIsNotAFile(t *testing.T) {
	f := playRunner("34")
	code, out, _ := run(t, f, "play", t.TempDir())
	if code != 1 || errCode(t, out) != "file_not_found" || len(f.calls) != 0 ||
		!strings.Contains(out, "directory") {
		t.Fatalf("exit %d: %s calls %v", code, out, f.calls)
	}
}

func TestPlayProRequired(t *testing.T) {
	f := playRunner("34")
	f.responses[provider("load")] = resp{out: bundle(`{"error":{"code":"pro_required","message":"Pro"}}`)}
	code, out, _ := run(t, f, "play", gpxFile(t))
	if code != 4 || errCode(t, out) != "pro_required" || f.called("am start") {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPlayNeverStarts(t *testing.T) {
	f := playRunner("34", "idle")
	f.sequences[provider("status")] = append(f.sequences[provider("status")][:1],
		resp{out: bundle(failedStatus)})
	reads := 0
	f.onCall = func(key string) {
		if key == provider("status") {
			reads++
		}
	}
	code, out, _ := runEnv(t, f, fakeClock, "play", gpxFile(t))
	if code != 7 || errCode(t, out) != "start_failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
	e, _ := decode(t, out)["error"].(map[string]any)
	last, _ := e["last_status"].(map[string]any)
	lastErr, _ := last["error"].(map[string]any)
	if last["state"] != "failed" || lastErr["code"] != "gps_lost" {
		t.Fatalf("last_status missing: %s", out)
	}
	// One setup read, then a read every 500 ms from 0 s to 5 s included.
	if reads != 1+11 {
		t.Fatalf("status reads = %d, want 12", reads)
	}
}

func TestPlayWaitArrives(t *testing.T) {
	f := playRunner("34", "simulating", "arrived")
	code, out, _ := runEnv(t, f, fakeClock, "play", "--wait", gpxFile(t))
	if code != 0 || decode(t, out)["state"] != "arrived" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPlayWaitThroughPause(t *testing.T) {
	f := playRunner("34", "simulating", "paused", "paused", "arrived")
	code, out, _ := runEnv(t, f, fakeClock, "play", "--wait", gpxFile(t))
	m := decode(t, out)
	if code != 0 || m["state"] != "arrived" || m["pace_source"] != "gpx_times" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPlayWaitStopped(t *testing.T) {
	f := playRunner("34", "simulating", "idle")
	code, out, _ := runEnv(t, f, fakeClock, "play", "--wait", gpxFile(t))
	if code != 7 || errCode(t, out) != "stopped" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPlayWaitFailed(t *testing.T) {
	f := playRunner("34", "simulating")
	f.sequences[provider("status")] = append(f.sequences[provider("status")],
		resp{out: bundle(`{"state":"failed","error":{"code":"gps_lost","message":"x"},` + ready + `}`)})
	code, out, _ := runEnv(t, f, fakeClock, "play", "--wait", gpxFile(t))
	e, _ := decode(t, out)["error"].(map[string]any)
	appErr, _ := e["app_error"].(map[string]any)
	if code != 7 || e["code"] != "mock_failed" || appErr["code"] != "gps_lost" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPlayWaitTimeout(t *testing.T) {
	f := playRunner("34", "simulating")
	code, out, _ := runEnv(t, f, fakeClock, "play", "--wait", "--timeout", "10s", gpxFile(t))
	if code != 8 || errCode(t, out) != "timeout" {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !strings.Contains(out, "continues") {
		t.Fatalf("message should say the simulation continues: %s", out)
	}
}

func TestPlayFlagsAfterPath(t *testing.T) {
	f := playRunner("34", "simulating", "arrived")
	f.responses[provider("load")+" --arg 40"] = resp{out: bundle(loadOK)}
	code, out, _ := runEnv(t, f, fakeClock, "play", gpxFile(t), "--speed", "40", "--wait")
	if code != 0 || !f.called("--method load --arg 40") || decode(t, out)["state"] != "arrived" {
		t.Fatalf("exit %d: %s calls %v", code, out, f.calls)
	}
}

func TestPlayOldAndroid(t *testing.T) {
	f := playRunner("27", "simulating")
	p := gpxFile(t)
	withTrack(f, p)
	code, out, _ := runEnv(t, f, fakeClock, "play", p)
	if code != 0 || !f.called("push "+p) || !f.called("--method load_pushed") {
		t.Fatalf("exit %d: %s calls %v", code, out, f.calls)
	}
}

const failedStatus = `{"state":"failed","error":{"code":"gps_lost","message":"x"},` + ready + `}`

func TestStatusFailedIsNotAnError(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("status")] = resp{out: bundle(failedStatus)}
	code, out, _ := run(t, f, "status")
	if code != 0 || decode(t, out)["state"] != "failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestStopAnsweringFailedIsNotAnError(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("stop")] = resp{out: bundle(failedStatus)}
	code, out, _ := run(t, f, "stop")
	if code != 0 || decode(t, out)["state"] != "failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestSetupWithFailedStateSucceeds(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("status")] = resp{out: bundle(failedStatus)}
	code, out, _ := run(t, f, "setup")
	if code != 0 || decode(t, out)["ready"] != true {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPlayProceedsPastStaleFailedState(t *testing.T) {
	f := playRunner("34", "simulating")
	f.sequences[provider("status")][0] = resp{out: bundle(failedStatus)}
	code, out, _ := runEnv(t, f, fakeClock, "play", gpxFile(t))
	if code != 0 || !f.called("content write") || !f.called("--method load") {
		t.Fatalf("exit %d: %s calls %v", code, out, f.calls)
	}
}
