package cli

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
	"github.com/asimonigh/mockroute-cli/internal/device"
)

type setupStep struct {
	name     string
	required bool
	commands func(pkg string) [][]string
}

// setupSteps lists the steps in the order of the app. developer_mode has no
// command: only the user can turn it on.
var setupSteps = []setupStep{
	{"location", true, func(pkg string) [][]string {
		return [][]string{
			{"pm", "grant", pkg, "android.permission.ACCESS_FINE_LOCATION"},
			{"pm", "grant", pkg, "android.permission.ACCESS_COARSE_LOCATION"},
		}
	}},
	{"notifications", false, func(pkg string) [][]string {
		return [][]string{{"pm", "grant", pkg, "android.permission.POST_NOTIFICATIONS"}}
	}},
	{"developer_mode", true, nil},
	{"mock_app", true, func(pkg string) [][]string {
		return [][]string{{"appops", "set", pkg, "android:mock_location", "allow"}}
	}},
	{"battery", false, func(pkg string) [][]string {
		return [][]string{{"dumpsys", "deviceidle", "whitelist", "+" + pkg}}
	}},
}

func runSetupCommand(ctx context.Context, g globalFlags, env Env) result {
	c, _, err := connect(ctx, g, env)
	if err != nil {
		return result{err: err}
	}
	out, err := runSetup(ctx, c)
	return result{value: out, err: err}
}

type setupStatus struct {
	Setup struct {
		Ready   bool     `json:"ready"`
		Missing []string `json:"missing"`
	} `json:"setup"`
}

func readSetup(ctx context.Context, c *device.Client) (setupStatus, error) {
	var s setupStatus
	raw, err := c.Call(ctx, "status", "")
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, clierr.New("protocol_error", "invalid status answer: %v", err)
	}
	return s, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// runSetup applies what adb can grant, then reports every step as done, applied or failed.
func runSetup(ctx context.Context, c *device.Client) (map[string]any, error) {
	initial, err := readSetup(ctx, c)
	if err != nil {
		return nil, err
	}
	steps := map[string]any{}
	errs := map[string]any{}
	ran := false
	for _, st := range setupSteps {
		if !contains(initial.Setup.Missing, st.name) {
			steps[st.name] = "done"
			continue
		}
		if st.commands == nil {
			continue
		}
		for _, cmd := range st.commands(c.Package) {
			ran = true
			out, runErr := c.Runner.Run(ctx, nil, append([]string{"-s", c.Serial, "shell"}, cmd...)...)
			if runErr != nil || strings.Contains(out, "Exception") {
				if _, seen := errs[st.name]; !seen {
					msg := strings.TrimSpace(out)
					if msg == "" && runErr != nil {
						msg = runErr.Error()
					}
					errs[st.name] = msg
				}
			}
		}
	}
	final := initial
	if ran {
		if final, err = readSetup(ctx, c); err != nil {
			return nil, err
		}
	}
	for _, st := range setupSteps {
		if _, done := steps[st.name]; done {
			continue
		}
		if contains(final.Setup.Missing, st.name) {
			steps[st.name] = "failed"
			if _, ok := errs[st.name]; !ok && st.commands == nil {
				errs[st.name] = "cannot be done through adb: enable it on the phone"
			}
		} else {
			steps[st.name] = "applied"
		}
	}
	result := map[string]any{"ready": final.Setup.Ready, "steps": steps, "errors": errs}
	for _, st := range setupSteps {
		if st.required && steps[st.name] == "failed" {
			e := clierr.New("setup_failed", "MockRoute setup is incomplete. On MIUI and HyperOS, turn on \"USB debugging (Security settings)\" in the developer options, then run setup again.")
			e.Details = map[string]any{"steps": steps, "errors": errs}
			return result, e
		}
	}
	return result, nil
}
