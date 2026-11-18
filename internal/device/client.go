package device

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/asimonigh/mockroute-cli/internal/adb"
	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

// ProtocolVersion is the adb protocol version this CLI speaks.
const ProtocolVersion = 1

// StartActivity is the app activity that brings the app up for CLI calls.
const StartActivity = "com.simonigh.fakemygpslocation.cli.CliStartActivity"

var providerMissing = []string{"Unknown authority", "Could not find provider", "Unknown URL"}

func providerMissingIn(out string) bool {
	for _, p := range providerMissing {
		if strings.Contains(out, p) {
			return true
		}
	}
	return false
}

func errAppOutdated() *clierr.Error {
	return clierr.New("app_outdated", "This MockRoute version has no CLI support. Update MockRoute.")
}

var resultPattern = regexp.MustCompile(`result=([A-Za-z0-9+/=]+)`)

// Client talks to the CLI content provider of one MockRoute install.
type Client struct {
	Runner  adb.Runner
	Serial  string
	Package string
}

// AppVersion is the answer of the version method.
type AppVersion struct {
	Protocol   int    `json:"protocol"`
	AppVersion string `json:"app_version"`
	BuildType  string `json:"build_type"`
	Package    string `json:"package"`
}

// Call invokes a provider method and returns the decoded JSON result.
// An error answer of the app becomes a *clierr.Error with the app's code.
func (c *Client) Call(ctx context.Context, method, arg string) (json.RawMessage, error) {
	raw, err := c.call(ctx, method, arg)
	if err != nil {
		return nil, err
	}
	if ce := appError(raw); ce != nil {
		return nil, ce
	}
	return raw, nil
}

// StatusCall runs a method that answers a status object (status, stop). Unlike
// Call, an "error" object next to a "state" is part of the answer (state
// failed), not a failed call.
func (c *Client) StatusCall(ctx context.Context, method string) (json.RawMessage, error) {
	raw, err := c.call(ctx, method, "")
	if err != nil {
		return nil, err
	}
	var probe struct {
		State string `json:"state"`
	}
	if json.Unmarshal(raw, &probe) == nil && probe.State != "" {
		return raw, nil
	}
	if ce := appError(raw); ce != nil {
		return nil, ce
	}
	return raw, nil
}

// call runs the provider method and decodes the base64 JSON result.
func (c *Client) call(ctx context.Context, method, arg string) (json.RawMessage, error) {
	args := []string{"-s", c.Serial, "shell", "content", "call", "--uri", "content://" + c.Package + ".cli", "--method", method}
	if arg != "" {
		args = append(args, "--arg", arg)
	}
	out, err := c.Runner.Run(ctx, nil, args...)
	m := resultPattern.FindStringSubmatch(out)
	if m == nil {
		if providerMissingIn(out) {
			return nil, errAppOutdated()
		}
		if err != nil {
			return nil, adb.RunError(ctx, out, err)
		}
		return nil, clierr.New("protocol_error", "unexpected answer from the app: %s", strings.TrimSpace(out))
	}
	raw, decErr := base64.StdEncoding.DecodeString(m[1])
	if decErr != nil {
		return nil, clierr.New("protocol_error", "invalid base64 in the app answer: %v", decErr)
	}
	if !json.Valid(raw) {
		return nil, clierr.New("protocol_error", "invalid JSON in the app answer: %s", strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

// appError turns an {"error":{...}} answer of the app into a *clierr.Error.
func appError(raw json.RawMessage) *clierr.Error {
	var probe struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Target  string `json:"target"`
			Missing []any  `json:"missing"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &probe) != nil || probe.Error == nil {
		return nil
	}
	e := probe.Error
	ce := clierr.New(e.Code, "%s", e.Message)
	if e.Target != "" || e.Missing != nil {
		ce.Details = map[string]any{}
		if e.Target != "" {
			ce.Details["target"] = e.Target
		}
		if e.Missing != nil {
			ce.Details["missing"] = e.Missing
		}
	}
	return ce
}

// Version asks the app for its version and checks the protocol is compatible.
func (c *Client) Version(ctx context.Context) (AppVersion, error) {
	raw, err := c.Call(ctx, "version", "")
	if err != nil {
		return AppVersion{}, err
	}
	var v AppVersion
	if err := json.Unmarshal(raw, &v); err != nil {
		return AppVersion{}, clierr.New("protocol_error", "invalid version answer: %v", err)
	}
	switch {
	case v.Protocol > ProtocolVersion:
		return v, clierr.New("cli_outdated", "MockRoute speaks protocol %d, this CLI speaks %d. Update the CLI.", v.Protocol, ProtocolVersion)
	case v.Protocol < ProtocolVersion:
		return v, clierr.New("app_outdated", "MockRoute speaks protocol %d, this CLI needs %d. Update MockRoute.", v.Protocol, ProtocolVersion)
	}
	return v, nil
}

// SDKInt returns the Android API level of the device.
func (c *Client) SDKInt(ctx context.Context) (int, error) {
	out, err := c.Runner.Run(ctx, nil, "-s", c.Serial, "shell", "getprop", "ro.build.version.sdk")
	if err != nil {
		return 0, adb.RunError(ctx, out, err)
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(out))
	if convErr != nil {
		return 0, clierr.New("protocol_error", "cannot read the Android version: %q", strings.TrimSpace(out))
	}
	return n, nil
}

// SendGPX hands a GPX file to the app and returns the provider method that loads it:
// "load" after a content write (Android 9+), "load_pushed" after an adb push.
func (c *Client) SendGPX(ctx context.Context, path string, sdk int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", clierr.New("file_not_found", "file not found: %s", path)
		}
		return "", clierr.New("file_not_found", "cannot open %s: %v", path, err)
	}
	defer file.Close()

	if sdk < 28 {
		out, err := c.Runner.Run(ctx, nil, "-s", c.Serial, "push", path, "/sdcard/Android/data/"+c.Package+"/files/cli/route.gpx")
		if err != nil {
			return "", adb.RunError(ctx, out, err)
		}
		return "load_pushed", nil
	}
	out, err := c.Runner.Run(ctx, file, "-s", c.Serial, "shell", "content", "write", "--uri", "content://"+c.Package+".cli/gpx")
	switch {
	case providerMissingIn(out):
		return "", errAppOutdated()
	case strings.Contains(out, "Exception") || strings.Contains(out, "Error"):
		return "", clierr.New("protocol_error", "sending the GPX failed: %s", strings.TrimSpace(out))
	case err != nil:
		return "", adb.RunError(ctx, out, err)
	}
	return "load", nil
}

// startTimeout bounds the am start call, which can hang on some Android versions.
var startTimeout = 15 * time.Second

// Start brings the app up so that it can answer CLI calls.
func (c *Client) Start(ctx context.Context) error {
	startCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	out, err := c.Runner.Run(startCtx, nil, "-s", c.Serial, "shell", "am", "start", "-n", c.Package+"/"+StartActivity)
	if strings.Contains(out, "Error:") || strings.Contains(out, "Exception") {
		return clierr.New("start_failed", "cannot start MockRoute: %s", strings.TrimSpace(out))
	}
	if err != nil {
		return adb.RunError(startCtx, out, err)
	}
	return nil
}
