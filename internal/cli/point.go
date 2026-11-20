package cli

import (
	"context"
	"encoding/json"
	"flag"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

const maxAccuracyM = 10000

// accuracyGiven reports whether --accuracy was written, so that an explicit 0 is not "absent".
func accuracyGiven(fs *flag.FlagSet) bool {
	given := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "accuracy" {
			given = true
		}
	})
	return given
}

func pointCommand(ctx context.Context, g globalFlags, env Env, accuracySet bool, args []string) result {
	lat, lon, err := ParseCoordinates(args)
	if err != nil {
		return result{err: err}
	}
	if accuracySet && !(g.Accuracy > 0 && g.Accuracy <= maxAccuracyM) {
		return result{err: clierr.New("invalid_accuracy", "--accuracy must be above 0 and at most %d meters", maxAccuracyM)}
	}

	c, _, err := connect(ctx, g, env)
	if err != nil {
		return result{err: err}
	}
	if _, err := runSetup(ctx, c); err != nil {
		return result{err: err}
	}
	answer, err := c.Call(ctx, "point", formatPointArg(lat, lon, g.Accuracy, accuracySet))
	if err != nil {
		if ce, ok := err.(*clierr.Error); ok && ce.Code == "unknown_method" {
			return result{err: clierr.New("app_outdated", "This MockRoute version cannot set a fixed point from the CLI. Update MockRoute.")}
		}
		return result{err: err}
	}
	var probe struct {
		State string `json:"state"`
	}
	if json.Unmarshal(answer, &probe) == nil && probe.State != "" {
		// A fixed point was already running and has been moved.
		if probe.State != "static" {
			e := clierr.New("start_failed", "MockRoute did not hold the fixed point (state %q)", probe.State)
			e.Details = map[string]any{"last_status": json.RawMessage(answer)}
			return result{err: e}
		}
		var v any
		if err := json.Unmarshal(answer, &v); err != nil {
			return result{err: clierr.New("protocol_error", "invalid point answer: %v", err)}
		}
		return result{value: v}
	}
	before, _, err := readStatus(ctx, c)
	if err != nil {
		return result{err: err}
	}
	if err := c.Start(ctx); err != nil {
		return result{err: err}
	}
	_, raw, err := awaitStart(ctx, c, env, before, pointPollLimit, "static")
	if err != nil {
		return result{err: err}
	}
	return result{value: json.RawMessage(raw)}
}
