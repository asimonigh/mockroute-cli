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

type playStatus struct {
	State string          `json:"state"`
	Error json.RawMessage `json:"error"`
}

func readStatus(ctx context.Context, c *device.Client) (playStatus, json.RawMessage, error) {
	raw, err := c.StatusCall(ctx, "status")
	if err != nil {
		return playStatus{}, nil, err
	}
	var s playStatus
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, nil, clierr.New("protocol_error", "invalid status answer: %v", err)
	}
	return s, raw, nil
}

func playCommand(ctx context.Context, g globalFlags, env Env, speedSet bool, args []string) result {
	if len(args) != 1 {
		return result{err: clierr.New("usage", "play needs exactly one GPX file")}
	}
	path := args[0]
	if info, err := os.Stat(path); err != nil {
		return result{err: clierr.New("file_not_found", "cannot open %s: %v", path, err)}
	} else if info.IsDir() {
		return result{err: clierr.New("file_not_found", "%s is a directory, not a GPX file", path)}
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
	sdk, err := c.SDKInt(ctx)
	if err != nil {
		return result{err: err}
	}
	loadMethod, err := c.SendGPX(ctx, path, sdk)
	if err != nil {
		return result{err: err}
	}
	loaded, err := c.Call(ctx, loadMethod, speedArg)
	if err != nil {
		return result{err: err}
	}
	var load struct {
		PaceSource json.RawMessage `json:"pace_source"`
	}
	if err := json.Unmarshal(loaded, &load); err != nil {
		return result{err: clierr.New("protocol_error", "invalid load answer: %v", err)}
	}
	if err := c.Start(ctx); err != nil {
		return result{err: err}
	}

	st, raw, err := awaitStart(ctx, c, env)
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
			case "idle":
				return result{err: clierr.New("stopped", "the simulation was stopped before arrival")}
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

// awaitStart polls status until the simulation runs, for at most startPollLimit.
func awaitStart(ctx context.Context, c *device.Client, env Env) (playStatus, json.RawMessage, error) {
	deadline := env.Now().Add(startPollLimit)
	for {
		st, raw, err := readStatus(ctx, c)
		if err != nil {
			return st, nil, err
		}
		switch st.State {
		case "simulating", "paused", "arrived":
			return st, raw, nil
		}
		if !env.Now().Before(deadline) {
			e := clierr.New("start_failed", "the simulation did not start within %s (state %q)", startPollLimit, st.State)
			e.Details = map[string]any{"last_status": raw}
			return st, nil, e
		}
		env.Sleep(startPollInterval)
	}
}
