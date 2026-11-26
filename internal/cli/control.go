package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

// controlCall sends a control method (speed, pause, resume) and returns the status answer.
// None of them runs setup: they act on a route that is already there.
func controlCall(ctx context.Context, g globalFlags, env Env, method, arg, what string) result {
	c, _, err := connect(ctx, g, env)
	if err != nil {
		return result{err: err}
	}
	raw, err := c.StatusCall(ctx, method, arg)
	if err != nil {
		return result{err: newMethodError(err, what)}
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return result{err: clierr.New("protocol_error", "invalid JSON in the app answer: %v", err)}
	}
	return result{value: json.RawMessage(buf.Bytes())}
}

func speedCommand(ctx context.Context, g globalFlags, env Env, args []string) result {
	if len(args) != 1 {
		return result{err: clierr.New("usage", "speed needs exactly one value in km/h")}
	}
	v, err := strconv.ParseFloat(args[0], 64)
	if err != nil || !(v >= minSpeedKmh && v <= maxSpeedKmh) {
		return result{err: clierr.New("invalid_speed", "speed must be between %d and %d km/h", minSpeedKmh, maxSpeedKmh)}
	}
	return controlCall(ctx, g, env, "speed", strconv.FormatFloat(v, 'f', -1, 64), "change the speed")
}

func pauseResumeCommand(ctx context.Context, g globalFlags, env Env, method, what string, args []string) result {
	if len(args) != 0 {
		return result{err: clierr.New("usage", "%s takes no argument", method)}
	}
	return controlCall(ctx, g, env, method, "", what)
}
