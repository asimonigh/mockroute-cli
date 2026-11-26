package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
	"github.com/asimonigh/mockroute-cli/internal/device"
)

// savedRoute is the part of a saved route the CLI needs to resolve a name.
type savedRoute struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// newMethodError maps unknown_method (an app without the method) to app_outdated.
func newMethodError(err error, what string) error {
	if ce, ok := err.(*clierr.Error); ok && ce.Code == "unknown_method" {
		return clierr.New("app_outdated", "This MockRoute version cannot %s from the CLI. Update MockRoute.", what)
	}
	return err
}

// resolveRoute returns the id of the route the user asked for: an identical id first, then the
// names equal under strings.EqualFold. Several names match: ambiguous_route, with their ids.
func resolveRoute(routes []savedRoute, query string) (string, error) {
	for _, r := range routes {
		if r.ID == query {
			return r.ID, nil
		}
	}
	var ids []string
	for _, r := range routes {
		if strings.EqualFold(r.Name, query) {
			ids = append(ids, r.ID)
		}
	}
	switch len(ids) {
	case 0:
		return "", clierr.New("route_not_found", "no saved route has the id or name %q; run mockroute routes to list them", query)
	case 1:
		return ids[0], nil
	}
	e := clierr.New("ambiguous_route", "%d saved routes are named %q; pass one of their ids to --route", len(ids), query)
	e.Details = map[string]any{"ids": ids}
	return "", e
}

func routesCommand(ctx context.Context, g globalFlags, env Env) result {
	c, _, err := connect(ctx, g, env)
	if err != nil {
		return result{err: err}
	}
	raw, err := c.Call(ctx, "routes", "")
	if err != nil {
		return result{err: newMethodError(err, "list saved routes")}
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return result{err: clierr.New("protocol_error", "invalid JSON in the app answer: %v", err)}
	}
	return result{value: json.RawMessage(buf.Bytes())}
}

// loadSavedRoute resolves [query] against the saved routes and asks the app to prepare the route.
// Only the id goes to the phone, never the name.
func loadSavedRoute(ctx context.Context, c *device.Client, query, speedArg string) (json.RawMessage, error) {
	raw, err := c.Call(ctx, "routes", "")
	if err != nil {
		return nil, newMethodError(err, "play a saved route")
	}
	var list struct {
		Routes []savedRoute `json:"routes"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, clierr.New("protocol_error", "invalid routes answer: %v", err)
	}
	id, err := resolveRoute(list.Routes, query)
	if err != nil {
		return nil, err
	}
	arg := id
	if speedArg != "" {
		arg = id + "," + speedArg
	}
	loaded, err := c.Call(ctx, "play_saved", arg)
	if err != nil {
		return nil, newMethodError(err, "play a saved route")
	}
	return loaded, nil
}
