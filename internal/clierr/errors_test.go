package clierr

import "testing"

func TestExitCodes(t *testing.T) {
	cases := map[int][]string{
		1: {"usage", "file_not_found"},
		2: {"adb_not_found", "no_device", "multiple_devices", "device_unauthorized", "device_locked"},
		3: {"app_not_installed", "multiple_packages", "app_outdated", "cli_outdated", "protocol_error", "unknown_method", "something_unknown"},
		4: {"pro_required"},
		5: {"setup_incomplete", "setup_failed"},
		6: {"invalid_gpx", "invalid_speed"},
		7: {"start_failed", "mock_failed", "stopped"},
		8: {"timeout"},
	}
	for want, codes := range cases {
		for _, c := range codes {
			e := New(c, "msg %d", 1)
			if got := e.ExitCode(); got != want {
				t.Errorf("%s: exit code = %d, want %d", c, got, want)
			}
			if e.Error() != "msg 1" || e.Code != c {
				t.Errorf("%s: unexpected error %q / code %q", c, e.Error(), e.Code)
			}
		}
	}
}
