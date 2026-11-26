package cli

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"strconv"
	"time"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
	"github.com/asimonigh/mockroute-cli/internal/device"
)

const (
	startPollInterval = 500 * time.Millisecond
	startPollLimit    = 5 * time.Second
	pointPollLimit    = 12 * time.Second // a point started after a route makes up to three 3 s fused calls
	waitPollInterval  = 2 * time.Second
	minSpeedKmh       = 1
	maxSpeedKmh       = 300
)

// speedGiven reports whether --speed was written, so that an explicit 0 is not "absent".
func speedGiven(fs *flag.FlagSet) bool {
	given := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "speed" {
			given = true
		}
	})
	return given
}

// routeGiven reports whether --route was written, so that an empty --route is an error, not "absent".
func routeGiven(fs *flag.FlagSet) bool {
	given := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "route" {
			given = true
		}
	})
	return given
}

type playStatus struct {
	State     string          `json:"state"`
	StartedAt string          `json:"started_at"`
	Error     json.RawMessage `json:"error"`
}

func readStatus(ctx context.Context, c *device.Client) (playStatus, json.RawMessage, error) {
	raw, err := c.StatusCall(ctx, "status", "")
	if err != nil {
		return playStatus{}, nil, err
	}
	var s playStatus
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, nil, clierr.New("protocol_error", "invalid status answer: %v", err)
	}
	return s, raw, nil
}

func playCommand(ctx context.Context, g globalFlags, env Env, speedSet, routeSet bool, args []string) result {
	if routeSet && g.Route == "" {
		return result{err: clierr.New("usage", "--route needs a saved route name or id")}
	}
	if g.Route != "" {
		if len(args) != 0 {
			return result{err: clierr.New("usage", "play takes a GPX file or --route, not both")}
		}
	} else if len(args) != 1 {
		return result{err: clierr.New("usage", "play needs exactly one GPX file")}
	}
	if g.Route == "" {
		path := args[0]
		if info, err := os.Stat(path); err != nil {
			return result{err: clierr.New("file_not_found", "cannot open %s: %v", path, err)}
		} else if info.IsDir() {
			return result{err: clierr.New("file_not_found", "%s is a directory, not a GPX file", path)}
		}
	}
	speedArg := ""
	if speedSet {
		if !(g.Speed >= minSpeedKmh && g.Speed <= maxSpeedKmh) {
			return result{err: clierr.New("invalid_speed", "--speed must be between %d and %d km/h", minSpeedKmh, maxSpeedKmh)}
		}
		speedArg = strconv.FormatFloat(g.Speed, 'f', -1, 64)
	}

	c, _, err := connect(ctx, g, env)
	if err != nil {
		return result{err: err}
	}
	if _, err := runSetup(ctx, c); err != nil {
		return result{err: err}
	}
	var loaded json.RawMessage
	if g.Route != "" {
		loaded, err = loadSavedRoute(ctx, c, g.Route, speedArg)
	} else {
		loaded, err = loadGPX(ctx, c, args[0], speedArg)
	}
	if err != nil {
		return result{err: err}
	}
	return startAndReport(ctx, g, env, c, loaded)
}

func loadGPX(ctx context.Context, c *device.Client, path, speedArg string) (json.RawMessage, error) {
	sdk, err := c.SDKInt(ctx)
	if err != nil {
		return nil, err
	}
	loadMethod, err := c.SendGPX(ctx, path, sdk)
	if err != nil {
		return nil, err
	}
	return c.Call(ctx, loadMethod, speedArg)
}

// startAndReport starts what [loaded] prepared, waits for the simulation (and for its arrival with
// --wait) and builds the answer: the status plus the pace_source of the load answer.
func startAndReport(ctx context.Context, g globalFlags, env Env, c *device.Client, loaded json.RawMessage) result {
	var load struct {
		PaceSource json.RawMessage `json:"pace_source"`
	}
	if err := json.Unmarshal(loaded, &load); err != nil {
		return result{err: clierr.New("protocol_error", "invalid load answer: %v", err)}
	}
	// What was running before: an active state left over (an arrived route is held until stop) must not pass for the new start
	before, _, err := readStatus(ctx, c)
	if err != nil {
		return result{err: err}
	}
	if err := c.Start(ctx); err != nil {
		return result{err: err}
	}

	st, raw, err := awaitStart(ctx, c, env, before, startPollLimit, "simulating", "paused", "arrived")
	if err != nil {
		return result{err: err}
	}
	if g.Wait {
		deadline := env.Now().Add(g.Timeout)
		for st.State != "arrived" {
			switch st.State {
			case "failed":
				e := clierr.New("mock_failed", "the simulation failed")
				if len(st.Error) > 0 {
					var appErr any
					if json.Unmarshal(st.Error, &appErr) == nil {
						e.Details = map[string]any{"app_error": appErr}
					}
				}
				return result{err: e}
			case "idle", "static":
				return result{err: clierr.New("stopped", "the simulation was stopped before arrival")}
			case "simulating", "paused":
			default:
				e := clierr.New("protocol_error", "unexpected state %q while waiting", st.State)
				e.Details = map[string]any{"last_status": raw}
				return result{err: e}
			}
			if g.Timeout > 0 && !env.Now().Before(deadline) {
				return result{err: clierr.New("timeout", "timed out after %s; the simulation continues", g.Timeout)}
			}
			env.Sleep(waitPollInterval)
			if st, raw, err = readStatus(ctx, c); err != nil {
				return result{err: err}
			}
		}
	}

	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return result{err: clierr.New("protocol_error", "invalid status answer: %v", err)}
	}
	if len(load.PaceSource) > 0 {
		fields["pace_source"] = load.PaceSource
	}
	return result{value: fields}
}

func isAccepted(state string, accepted []string) bool {
	for _, a := range accepted {
		if state == a {
			return true
		}
	}
	return false
}

// awaitStart polls status until the state is one of [accepted], for at most [limit]. When [before] was
// already in an accepted state, only a different started_at proves that this start happened.
func awaitStart(ctx context.Context, c *device.Client, env Env, before playStatus, limit time.Duration, accepted ...string) (playStatus, json.RawMessage, error) {
	deadline := env.Now().Add(limit)
	for {
		st, raw, err := readStatus(ctx, c)
		if err != nil {
			return st, nil, err
		}
		if isAccepted(st.State, accepted) && (!isAccepted(before.State, accepted) || st.StartedAt != before.StartedAt) {
			return st, raw, nil
		}
		if !env.Now().Before(deadline) {
			e := clierr.New("start_failed", "MockRoute did not start within %s (state %q)", limit, st.State)
			e.Details = map[string]any{"last_status": raw}
			return st, nil, e
		}
		env.Sleep(startPollInterval)
	}
}
