package cli

import (
	"errors"
	"strings"
	"testing"
)

func statusJSON(ready bool, missing ...string) string {
	quoted := make([]string, len(missing))
	for i, m := range missing {
		quoted[i] = `"` + m + `"`
	}
	r := "false"
	if ready {
		r = "true"
	}
	return `{"state":"idle","setup":{"ready":` + r + `,"missing":[` + strings.Join(quoted, ",") + `]}}`
}

func setupResponses(statuses ...string) (*fakeRunner, *int) {
	f := &fakeRunner{responses: baseResponses()}
	n := 0
	// The fake answers by key, so the status answer switches after the first read through this wrapper.
	f.responses[provider("status")] = resp{out: bundle(statuses[0])}
	if len(statuses) > 1 {
		f.onCall = func(key string) {
			if key == provider("status") {
				n++
				if n >= 1 {
					f.responses[key] = resp{out: bundle(statuses[1])}
				}
			}
		}
	}
	return f, &n
}

func TestSetupAllDone(t *testing.T) {
	f, _ := setupResponses(statusJSON(true))
	code, out, _ := run(t, f, "setup")
	m := decode(t, out)
	if code != 0 || m["ready"] != true {
		t.Fatalf("exit %d: %s", code, out)
	}
	if f.called("pm grant") || f.called("appops") || f.called("deviceidle") {
		t.Fatalf("unexpected setup call: %v", f.calls)
	}
	steps := m["steps"].(map[string]any)
	for _, s := range []string{"location", "notifications", "developer_mode", "mock_app", "battery"} {
		if steps[s] != "done" {
			t.Fatalf("step %s = %v", s, steps[s])
		}
	}
}

func TestSetupAppliesMockApp(t *testing.T) {
	f, _ := setupResponses(statusJSON(false, "mock_app"), statusJSON(true))
	f.responses["-s S1 shell appops set "+pkg+" android:mock_location allow"] = resp{}
	code, out, _ := run(t, f, "setup")
	m := decode(t, out)
	steps := m["steps"].(map[string]any)
	if code != 0 || steps["mock_app"] != "applied" || steps["location"] != "done" || m["ready"] != true {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestSetupLocationRefused(t *testing.T) {
	f, _ := setupResponses(statusJSON(false, "location"))
	f.responses["-s S1 shell pm grant "+pkg+" android.permission.ACCESS_FINE_LOCATION"] = resp{
		out: "Exception occurred while executing 'grant':\njava.lang.SecurityException: grant failed", err: errors.New("exit status 255"),
	}
	f.responses["-s S1 shell pm grant "+pkg+" android.permission.ACCESS_COARSE_LOCATION"] = resp{}
	code, out, _ := run(t, f, "setup")
	e := decode(t, out)["error"].(map[string]any)
	if code != 5 || e["code"] != "setup_failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
	if e["steps"].(map[string]any)["location"] != "failed" {
		t.Fatalf("steps: %v", e["steps"])
	}
	if !strings.Contains(e["errors"].(map[string]any)["location"].(string), "SecurityException") {
		t.Fatalf("errors: %v", e["errors"])
	}
	if !strings.Contains(e["message"].(string), "USB debugging (Security settings)") {
		t.Fatalf("message: %v", e["message"])
	}
}

func TestSetupDeveloperModeMissing(t *testing.T) {
	f, _ := setupResponses(statusJSON(false, "developer_mode"))
	code, out, _ := run(t, f, "setup")
	if code != 5 || errCode(t, out) != "setup_failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestSetupOptionalBatteryFails(t *testing.T) {
	f, _ := setupResponses(statusJSON(false, "battery"))
	f.responses["-s S1 shell dumpsys deviceidle whitelist +"+pkg] = resp{out: "nope", err: errors.New("exit status 1")}
	code, out, _ := run(t, f, "setup")
	m := decode(t, out)
	if code != 0 || m["steps"].(map[string]any)["battery"] != "failed" {
		t.Fatalf("exit %d: %s", code, out)
	}
}
