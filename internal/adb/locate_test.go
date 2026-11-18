package adb

import (
	"errors"
	"testing"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

func codeOf(err error) string {
	var ce *clierr.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func locator(goos string, env map[string]string, existing map[string]bool, path string) Locator {
	return Locator{
		Getenv: func(k string) string { return env[k] },
		GOOS:   goos,
		Home:   "/home/u",
		Exists: func(p string) bool { return existing[p] },
		LookPath: func(name string) (string, error) {
			if path != "" {
				return path, nil
			}
			return "", errors.New("not found")
		},
	}
}

func TestLocate(t *testing.T) {
	env := map[string]string{"ANDROID_HOME": "/ah", "ANDROID_SDK_ROOT": "/asr", "LOCALAPPDATA": `C:\L`}
	all := map[string]bool{
		"/explicit/adb": true, "/ah/platform-tools/adb": true, "/asr/platform-tools/adb": true,
		"/home/u/Library/Android/sdk/platform-tools/adb": true,
		"/home/u/Android/Sdk/platform-tools/adb":         true,
	}
	copyWithout := func(keys ...string) map[string]bool {
		m := map[string]bool{}
		for k, v := range all {
			m[k] = v
		}
		for _, k := range keys {
			delete(m, k)
		}
		return m
	}

	t.Run("explicit wins", func(t *testing.T) {
		got, err := locator("linux", env, all, "/bin/adb").Find("/explicit/adb")
		if err != nil || got != "/explicit/adb" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("explicit missing", func(t *testing.T) {
		_, err := locator("linux", env, all, "/bin/adb").Find("/nope/adb")
		if codeOf(err) != "adb_not_found" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("path next", func(t *testing.T) {
		got, err := locator("linux", env, all, "/bin/adb").Find("")
		if err != nil || got != "/bin/adb" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("android home next", func(t *testing.T) {
		got, _ := locator("linux", env, all, "").Find("")
		if got != "/ah/platform-tools/adb" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("sdk root next", func(t *testing.T) {
		got, _ := locator("linux", env, copyWithout("/ah/platform-tools/adb"), "").Find("")
		if got != "/asr/platform-tools/adb" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("darwin default", func(t *testing.T) {
		got, _ := locator("darwin", nil, all, "").Find("")
		if got != "/home/u/Library/Android/sdk/platform-tools/adb" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("linux default", func(t *testing.T) {
		got, _ := locator("linux", nil, all, "").Find("")
		if got != "/home/u/Android/Sdk/platform-tools/adb" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("windows default and exe", func(t *testing.T) {
		want := `C:\L\Android\Sdk\platform-tools\adb.exe`
		got, err := locator("windows", map[string]string{"LOCALAPPDATA": `C:\L`}, map[string]bool{want: true}, "").Find("")
		if err != nil || got != want {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("windows android home uses exe", func(t *testing.T) {
		got, _ := locator("windows", map[string]string{"ANDROID_HOME": `C:\sdk`}, map[string]bool{`C:\sdk\platform-tools\adb.exe`: true}, "").Find("")
		if got != `C:\sdk\platform-tools\adb.exe` {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("nothing", func(t *testing.T) {
		_, err := locator("linux", nil, nil, "").Find("")
		if codeOf(err) != "adb_not_found" {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestLocateSkipsOSDefaultWithoutHome(t *testing.T) {
	l := locator("darwin", nil, map[string]bool{"/Library/Android/sdk/platform-tools/adb": true}, "")
	l.Home = ""
	if _, err := l.Find(""); codeOf(err) != "adb_not_found" {
		t.Fatalf("want adb_not_found, got %v", err)
	}
}
