package cli

import (
	"reflect"
	"strings"
	"testing"
)

const startActivity = "-s S1 shell am start -n " + pkg + "/com.simonigh.fakemygpslocation.cli.CliStartActivity"

// pointRunner scripts a device ready for a fixed point: setup and before status read idle, then the given states.
func pointRunner(arg string, states ...string) *fakeRunner {
	f := &fakeRunner{responses: baseResponses(), sequences: map[string][]resp{}}
	seq := []resp{{out: bundle(stateJSON("idle"))}, {out: bundle(stateJSON("idle"))}}
	for _, s := range states {
		seq = append(seq, resp{out: bundle(stateJSON(s))})
	}
	f.sequences[provider("status")] = seq
	f.responses[provider("point")+" --arg "+arg] = resp{out: bundle(`{"staged":true}`)}
	f.responses[startActivity] = resp{out: "Status: ok\n"}
	return f
}

func TestPointStartsStaged(t *testing.T) {
	f := pointRunner("43.6045,1.444", "static")
	code, out, _ := runEnv(t, f, fakeClock, "point", "43.6045", "1.444")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	want := []string{
		"devices -l",
		"-s S1 shell pm list packages " + pkg,
		provider("version"),
		provider("status"),
		provider("point") + " --arg 43.6045,1.444",
		provider("status"),
		startActivity,
		provider("status"),
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls:\n%s", strings.Join(f.calls, "\n"))
	}
	if decode(t, out)["state"] != "static" {
		t.Fatalf("got %s", out)
	}
}

func TestPointMovesRunning(t *testing.T) {
	f := pointRunner("43.6045,1.444")
	f.responses[provider("point")+" --arg 43.6045,1.444"] = resp{out: bundle(`{"state":"static","static":{"lat":43.6045},` + ready + `}`)}
	code, out, _ := runEnv(t, f, fakeClock, "point", "43.6045,1.444")
	if code != 0 || f.called("am start") || decode(t, out)["state"] != "static" {
		t.Fatalf("exit %d: %s calls %v", code, out, f.calls)
	}
}

func TestPointAccuracyArg(t *testing.T) {
	f := pointRunner("43.6045,1.444,25", "static")
	if code, out, _ := runEnv(t, f, fakeClock, "point", "43.6045", "1.444", "--accuracy", "25"); code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !f.called("--method point --arg 43.6045,1.444,25") {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestPointInvalidAccuracy(t *testing.T) {
	for _, a := range []string{"0", "10001", "-5", "NaN"} {
		f := pointRunner("43.6045,1.444")
		code, out, _ := run(t, f, "point", "43.6045", "1.444", "--accuracy", a)
		if code != 6 || errCode(t, out) != "invalid_accuracy" || len(f.calls) != 0 {
			t.Fatalf("accuracy %s: exit %d %s calls %v", a, code, out, f.calls)
		}
	}
}

func TestPointInvalidCoordinates(t *testing.T) {
	for _, args := range [][]string{{"95", "1"}, {"abc"}, {"43.6"}, {}} {
		f := pointRunner("43.6045,1.444")
		code, out, _ := run(t, f, append([]string{"point"}, args...)...)
		if code != 6 || errCode(t, out) != "invalid_coordinates" || len(f.calls) != 0 {
			t.Fatalf("args %v: exit %d %s calls %v", args, code, out, f.calls)
		}
	}
}

func TestPointOldApp(t *testing.T) {
	f := pointRunner("43.6045,1.444")
	f.responses[provider("point")+" --arg 43.6045,1.444"] = resp{out: bundle(`{"error":{"code":"unknown_method","message":"x"}}`)}
	code, out, _ := runEnv(t, f, fakeClock, "point", "43.6045", "1.444")
	if code != 3 || errCode(t, out) != "app_outdated" || !strings.Contains(out, "cannot set a fixed point from the CLI") || f.called("am start") {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPointNeverStarts(t *testing.T) {
	f := pointRunner("43.6045,1.444", "idle")
	code, out, _ := runEnv(t, f, fakeClock, "point", "43.6045", "1.444")
	if code != 7 || errCode(t, out) != "start_failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
	e, _ := decode(t, out)["error"].(map[string]any)
	last, _ := e["last_status"].(map[string]any)
	if last["state"] != "idle" {
		t.Fatalf("last_status missing: %s", out)
	}
}

func TestPointNegativeLongitude(t *testing.T) {
	f := pointRunner("43.6045,-1.444", "static")
	if code, out, _ := runEnv(t, f, fakeClock, "point", "43.6045", "-1.444"); code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !f.called("--method point --arg 43.6045,-1.444") {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestPointNegativeForms(t *testing.T) {
	cases := []struct {
		args []string
		arg  string
	}{
		{[]string{"-33.86,151.2"}, "-33.86,151.2"},
		{[]string{"-33.86;151.2"}, "-33.86,151.2"},
		{[]string{"-33,86", "151,2"}, "-33.86,151.2"},
		{[]string{"43,6045", "-1,444"}, "43.6045,-1.444"},
	}
	for _, c := range cases {
		f := pointRunner(c.arg, "static")
		code, out, _ := runEnv(t, f, fakeClock, append([]string{"point"}, c.args...)...)
		if code != 0 || !f.called("--method point --arg "+c.arg) {
			t.Fatalf("%v: exit %d %s calls %v", c.args, code, out, f.calls)
		}
	}
}

func statusReads(f *fakeRunner) int {
	n := 0
	for _, c := range f.calls {
		if c == provider("status") {
			n++
		}
	}
	return n
}

func TestPointWaitsTwelveSeconds(t *testing.T) {
	f := pointRunner("43.6045,1.444", "idle")
	code, out, _ := runEnv(t, f, fakeClock, "point", "43.6045", "1.444")
	if code != 7 || errCode(t, out) != "start_failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
	// Two reads before the start, then one every 500 ms from 0 s to 12 s.
	if n := statusReads(f); n != 27 {
		t.Fatalf("status reads: %d", n)
	}
	if !strings.Contains(out, "within 12s") || strings.Contains(out, "simulation") {
		t.Fatalf("message: %s", out)
	}
}

func TestPointMoveAnswerNotStatic(t *testing.T) {
	f := pointRunner("43.6045,1.444")
	f.responses[provider("point")+" --arg 43.6045,1.444"] = resp{out: bundle(`{"state":"idle"}`)}
	code, out, _ := runEnv(t, f, fakeClock, "point", "43.6045", "1.444")
	if code != 7 || errCode(t, out) != "start_failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
	e, _ := decode(t, out)["error"].(map[string]any)
	last, _ := e["last_status"].(map[string]any)
	if last["state"] != "idle" {
		t.Fatalf("last_status missing: %s", out)
	}
}
