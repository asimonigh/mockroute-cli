// Package device finds the MockRoute variant installed on a device.
package device

import (
	"context"
	"slices"
	"strings"

	"github.com/asimonigh/mockroute-cli/internal/adb"
	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

// Packages lists the known MockRoute application ids.
var Packages = []string{
	"com.simonigh.fakemygpslocation",
	"com.simonigh.fakemygpslocation.internal",
	"com.simonigh.fakemygpslocation.debug",
	"com.simonigh.fakemygpslocation.staging",
}

// FindPackage returns the installed MockRoute variant, or explicit when it is installed.
func FindPackage(ctx context.Context, r adb.Runner, serial, explicit string) (string, error) {
	out, err := r.Run(ctx, nil, "-s", serial, "shell", "pm", "list", "packages", Packages[0])
	if err != nil {
		return "", clierr.New("app_not_installed", "cannot list packages on %s: %v", serial, err)
	}
	var installed []string
	for _, line := range strings.Split(out, "\n") {
		name, ok := strings.CutPrefix(strings.TrimSpace(line), "package:")
		if ok && slices.Contains(Packages, name) {
			installed = append(installed, name)
		}
	}
	if explicit != "" {
		if slices.Contains(installed, explicit) {
			return explicit, nil
		}
		return "", clierr.New("app_not_installed", "%s is not installed on %s", explicit, serial)
	}
	switch len(installed) {
	case 0:
		return "", clierr.New("app_not_installed", "MockRoute is not installed on %s", serial)
	case 1:
		return installed[0], nil
	}
	return "", clierr.New("multiple_packages", "several MockRoute variants installed (%s): pass --package", strings.Join(installed, ", "))
}
