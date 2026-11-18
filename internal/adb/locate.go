package adb

import (
	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

// Locator finds the adb binary. Its fields are injectable for tests.
type Locator struct {
	Getenv   func(string) string
	GOOS     string
	Home     string
	Exists   func(string) bool
	LookPath func(string) (string, error)
}

// Find returns the adb path: explicit, then PATH, ANDROID_HOME, ANDROID_SDK_ROOT
// and the default SDK location of the OS.
func (l Locator) Find(explicit string) (string, error) {
	if explicit != "" {
		if l.Exists(explicit) {
			return explicit, nil
		}
		return "", clierr.New("adb_not_found", "adb not found at %s", explicit)
	}
	if p, err := l.LookPath("adb"); err == nil && p != "" {
		return p, nil
	}
	sep, exe := "/", "adb"
	if l.GOOS == "windows" {
		sep, exe = `\`, "adb.exe"
	}
	candidates := []string{}
	for _, v := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if root := l.Getenv(v); root != "" {
			candidates = append(candidates, root+sep+"platform-tools"+sep+exe)
		}
	}
	switch {
	case l.Home == "" && l.GOOS != "windows":
	case l.GOOS == "darwin":
		candidates = append(candidates, l.Home+"/Library/Android/sdk/platform-tools/"+exe)
	case l.GOOS == "linux":
		candidates = append(candidates, l.Home+"/Android/Sdk/platform-tools/"+exe)
	case l.GOOS == "windows":
		if app := l.Getenv("LOCALAPPDATA"); app != "" {
			candidates = append(candidates, app+`\Android\Sdk\platform-tools\`+exe)
		}
	}
	for _, c := range candidates {
		if l.Exists(c) {
			return c, nil
		}
	}
	return "", clierr.New("adb_not_found", "adb not found: install Android platform-tools, set ANDROID_HOME or pass --adb")
}
