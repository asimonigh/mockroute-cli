package cli

import (
	"strings"
	"testing"
)

const pausedJSON = `{"state":"paused",` + ready + `}`

// controlRunner answers [method] with [answer], with or without the speed argument 40.
func controlRunner(method, answer string) *fakeRunner {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider(method)] = resp{out: bundle(answer)}
	f.responses[provider(method)+" --arg 40"] = resp{out: bundle(answer)}
	return f
}

func TestSpeedSendsArg(t *testing.T) {
	f := controlRunner("speed", pausedJSON)
	f.responses[provider("speed")+" --arg 42.5"] = resp{out: bundle(pausedJSON)}
	code, out, _ := run(t, f, "speed", "42.5")
	if code != 0 || decode(t, out)["state"] != "paused" || !f.called(provider("speed")+" --arg 42.5") {
		t.Fatalf("exit %d: %s calls %v", code, out, f.calls)
	}
	if f.called("-s S1 shell content call --uri content://"+pkg+".cli --method status") || f.called("am start") {
		t.Fatalf("unexpected calls: %v", f.calls)
	}
}

func TestSpeedInvalid(t *testing.T) {
	for _, s := range []string{"0", "301", "abc", "-5", "0.5"} {
		f := playRunner("34")
		code, out, _ := run(t, f, "speed", s)
		if code != 6 || errCode(t, out) != "invalid_speed" || len(f.calls) != 0 {
			t.Fatalf("speed %s: exit %d %s calls %v", s, code, out, f.calls)
		}
	}
}

func TestSpeedUsage(t *testing.T) {
	for _, args := range [][]string{{"speed"}, {"speed", "40", "50"}} {
		f := playRunner("34")
		code, out, _ := run(t, f, args...)
		if code != 1 || errCode(t, out) != "usage" || len(f.calls) != 0 {
			t.Fatalf("%v: exit %d %s calls %v", args, code, out, f.calls)
		}
	}
}

func TestSpeedNoRoute(t *testing.T) {
	f := controlRunner("speed", `{"error":{"code":"no_route","message":"No route is playing or paused"}}`)
	code, out, _ := run(t, f, "speed", "40")
	if code != 7 || errCode(t, out) != "no_route" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestSpeedProRequiredAndInvalid(t *testing.T) {
	f := controlRunner("speed", `{"error":{"code":"pro_required","message":"Pro"}}`)
	if code, out, _ := run(t, f, "speed", "40"); code != 4 || errCode(t, out) != "pro_required" {
		t.Fatalf("exit %d: %s", code, out)
	}
	g := controlRunner("speed", `{"error":{"code":"invalid_speed","message":"x"}}`)
	if code, out, _ := run(t, g, "speed", "40"); code != 6 || errCode(t, out) != "invalid_speed" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPauseResume(t *testing.T) {
	for _, m := range []string{"pause", "resume"} {
		f := controlRunner(m, pausedJSON)
		code, out, _ := run(t, f, m)
		if code != 0 || decode(t, out)["state"] != "paused" || !f.called(provider(m)) || f.called(provider(m)+" --arg") {
			t.Fatalf("%s: exit %d: %s calls %v", m, code, out, f.calls)
		}
		if f.called("-s S1 shell pm grant") {
			t.Fatalf("%s ran setup: %v", m, f.calls)
		}
		g := playRunner("34")
		code, out, _ = run(t, g, m, "extra")
		if code != 1 || errCode(t, out) != "usage" || len(g.calls) != 0 {
			t.Fatalf("%s extra: exit %d %s calls %v", m, code, out, g.calls)
		}
	}
}

func TestPauseNoRoute(t *testing.T) {
	f := controlRunner("pause", `{"error":{"code":"no_route","message":"No route is playing or paused"}}`)
	if code, out, _ := run(t, f, "pause"); code != 7 || errCode(t, out) != "no_route" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestControlOldApp(t *testing.T) {
	old := `{"error":{"code":"unknown_method","message":"x"}}`
	for _, c := range []struct {
		args         []string
		method, what string
	}{
		{[]string{"speed", "40"}, "speed", "change the speed"},
		{[]string{"pause"}, "pause", "pause a route"},
		{[]string{"resume"}, "resume", "resume a route"},
	} {
		f := controlRunner(c.method, old)
		code, out, _ := run(t, f, c.args...)
		if code != 3 || errCode(t, out) != "app_outdated" || !strings.Contains(out, "cannot "+c.what+" from the CLI") {
			t.Fatalf("%v: exit %d: %s", c.args, code, out)
		}
	}
}
