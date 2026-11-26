// Package cli implements the mockroute commands.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/asimonigh/mockroute-cli/internal/adb"
	"github.com/asimonigh/mockroute-cli/internal/clierr"
	"github.com/asimonigh/mockroute-cli/internal/device"
)

// Env holds what the commands need from the outside world, injectable for tests.
type Env struct {
	Stdout, Stderr io.Writer
	Locator        adb.Locator
	NewRunner      func(adbPath string) adb.Runner
	Sleep          func(time.Duration)
	Now            func() time.Time
	Version        string
}

type globalFlags struct {
	Serial, Package, Adb string
	Route                string
	Speed, Accuracy      float64
	Timeout              time.Duration
	Wait                 bool
}

const usage = `Usage: mockroute [--serial S] [--package P] [--adb PATH] <command>

Commands:
  setup     grant the permissions MockRoute needs and report what is missing
  status    print the state of MockRoute
  stop      stop the running simulation
  routes    list the saved routes
  play      play <file.gpx> [--speed km/h] [--wait] [--timeout duration]
  play      play --route <name|id> [--speed km/h] [--wait] [--timeout duration]
  point     point <lat> <lon> [--accuracy m]: hold a fixed position
  version   print the CLI and app versions
`

// newFlagSet declares every option of the CLI. Commands that take more options
// register them here so that reorder knows which ones consume a value.
func newFlagSet(g *globalFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("mockroute", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&g.Serial, "serial", "", "")
	fs.StringVar(&g.Package, "package", "", "")
	fs.StringVar(&g.Adb, "adb", "", "")
	fs.StringVar(&g.Route, "route", "", "")
	fs.Float64Var(&g.Speed, "speed", 0, "")
	fs.Float64Var(&g.Accuracy, "accuracy", 0, "")
	fs.DurationVar(&g.Timeout, "timeout", 0, "")
	fs.BoolVar(&g.Wait, "wait", false, "")
	return fs
}

// reorder moves the options before the positional arguments so that flag.Parse
// sees them all. Options declared with a value take the next argument unless
// written --opt=value; boolean options never do. Everything after "--" is positional.
func reorder(fs *flag.FlagSet, args []string) ([]string, error) {
	var opts, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" || startsNumber(a) {
			positional = append(positional, a)
			continue
		}
		opts = append(opts, a)
		if strings.Contains(a, "=") {
			continue
		}
		f := fs.Lookup(strings.TrimLeft(a, "-"))
		if f == nil {
			continue // flag.Parse reports the unknown option
		}
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("option %s needs a value", a)
		}
		i++
		opts = append(opts, args[i])
	}
	return append(opts, positional...), nil
}

// startsNumber reports whether a token that starts with "-" reads as a number, such as -33.86,
// -33,86 or -33.86;151.2: "-" followed by a digit or a dot. No option name starts with one.
func startsNumber(a string) bool {
	return len(a) > 1 && (a[1] >= '0' && a[1] <= '9' || a[1] == '.')
}

// Run executes the CLI and returns the process exit code.
func Run(args []string, env Env) int {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--help" || a == "-h" || a == "-help" {
			fmt.Fprint(env.Stderr, usage)
			return 1
		}
	}
	var g globalFlags
	fs := newFlagSet(&g)
	ordered, err := reorder(fs, args)
	if err == nil {
		err = fs.Parse(ordered)
	}
	if err != nil {
		return fail(env, clierr.New("usage", "%v", err))
	}
	if fs.NArg() == 0 {
		fmt.Fprint(env.Stderr, usage)
		return 1
	}
	ctx := context.Background()
	switch cmd := fs.Arg(0); cmd {
	case "setup":
		return finish(env, runSetupCommand(ctx, g, env))
	case "status", "stop":
		return finish(env, passThrough(ctx, g, env, cmd))
	case "routes":
		return finish(env, routesCommand(ctx, g, env))
	case "play":
		return finish(env, playCommand(ctx, g, env, speedGiven(fs), fs.Args()[1:]))
	case "point":
		return finish(env, pointCommand(ctx, g, env, accuracyGiven(fs), fs.Args()[1:]))
	case "version":
		return finish(env, versionCommand(ctx, g, env))
	default:
		return fail(env, clierr.New("usage", "unknown command %q", cmd))
	}
}

// result is a command outcome: a JSON value to print, or an error.
type result struct {
	value any
	err   error
}

func finish(env Env, r result) int {
	if r.err != nil {
		return fail(env, r.err)
	}
	return emit(env, r.value)
}

func emit(env Env, v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return fail(env, clierr.New("internal", "cannot encode the result: %v", err))
	}
	fmt.Fprintf(env.Stdout, "%s\n", b)
	return 0
}

// fail prints {"error":{"code","message",...details}} and returns the exit code.
func fail(env Env, err error) int {
	ce, ok := err.(*clierr.Error)
	if !ok {
		ce = clierr.New("internal", "%v", err)
	}
	body := map[string]any{}
	for k, v := range ce.Details {
		body[k] = v
	}
	body["code"] = ce.Code
	body["message"] = ce.Message
	b, _ := json.Marshal(map[string]any{"error": body})
	fmt.Fprintf(env.Stdout, "%s\n", b)
	return ce.ExitCode()
}

// connect locates adb, picks the device and the MockRoute variant, and checks the protocol.
// It returns the version the app reported. When only the protocol check fails
// (cli_outdated, app_outdated), the client and the version come back with the error.
func connect(ctx context.Context, g globalFlags, env Env) (*device.Client, device.AppVersion, error) {
	adbPath, err := env.Locator.Find(g.Adb)
	if err != nil {
		return nil, device.AppVersion{}, err
	}
	r := env.NewRunner(adbPath)
	out, runErr := r.Run(ctx, nil, "devices", "-l")
	if runErr != nil {
		return nil, device.AppVersion{}, adb.RunError(ctx, out, runErr)
	}
	dev, err := adb.SelectDevice(adb.ParseDevices(out), g.Serial)
	if err != nil {
		return nil, device.AppVersion{}, err
	}
	pkg, err := device.FindPackage(ctx, r, dev.Serial, g.Package)
	if err != nil {
		return nil, device.AppVersion{}, err
	}
	c := &device.Client{Runner: r, Serial: dev.Serial, Package: pkg}
	v, err := c.Version(ctx)
	return c, v, err
}

func passThrough(ctx context.Context, g globalFlags, env Env, method string) result {
	c, _, err := connect(ctx, g, env)
	if err != nil {
		return result{err: err}
	}
	raw, err := c.StatusCall(ctx, method, "")
	if err != nil {
		return result{err: err}
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return result{err: clierr.New("protocol_error", "invalid JSON in the app answer: %v", err)}
	}
	return result{value: json.RawMessage(buf.Bytes())}
}

func versionCommand(ctx context.Context, g globalFlags, env Env) result {
	out := map[string]any{"cli": env.Version}
	c, v, err := connect(ctx, g, env)
	var ce *clierr.Error
	// A newer app is worth showing: the version command is how a mismatch gets diagnosed.
	if c != nil && (err == nil || (errors.As(err, &ce) && ce.Code == "cli_outdated")) {
		out["app"] = v
	}
	return result{value: out}
}
