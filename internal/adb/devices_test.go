package adb

import (
	"strings"
	"testing"
)

func TestParseDevices(t *testing.T) {
	out := "* daemon not running; starting now at tcp:5037\r\n* daemon started successfully\r\nList of devices attached\r\n" +
		"R5CT123\tdevice usb:1-1 product:x model:Pixel_7 device:panther transport_id:1\r\n" +
		"\r\n" +
		"emulator-5554\tunauthorized\r\n" +
		"ABC\toffline\r\n\r\n"
	got := ParseDevices(out)
	want := []Device{{"R5CT123", "device"}, {"emulator-5554", "unauthorized"}, {"ABC", "offline"}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %v want %v", i, got[i], want[i])
		}
	}
	if len(ParseDevices("List of devices attached\n\n")) != 0 {
		t.Error("expected no devices")
	}
}

func TestSelectDevice(t *testing.T) {
	dev := func(s, st string) Device { return Device{s, st} }
	tests := []struct {
		name    string
		devices []Device
		serial  string
		want    string
		code    string
	}{
		{"none", nil, "", "", "no_device"},
		{"only unauthorized", []Device{dev("A", "unauthorized")}, "", "", "device_unauthorized"},
		{"two devices", []Device{dev("A", "device"), dev("B", "device")}, "", "", "multiple_devices"},
		{"unknown serial", []Device{dev("A", "device")}, "Z", "", "no_device"},
		{"serial unauthorized", []Device{dev("A", "unauthorized")}, "A", "", "device_unauthorized"},
		{"one among offline", []Device{dev("O", "offline"), dev("A", "device")}, "", "A", ""},
		{"serial chosen among two", []Device{dev("A", "device"), dev("B", "device")}, "B", "B", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SelectDevice(tc.devices, tc.serial)
			if codeOf(err) != tc.code {
				t.Fatalf("code = %q (%v), want %q", codeOf(err), err, tc.code)
			}
			if tc.code == "" && got.Serial != tc.want {
				t.Fatalf("got %v", got)
			}
		})
	}
	_, err := SelectDevice([]Device{dev("A", "device"), dev("B", "device")}, "")
	if !strings.Contains(err.Error(), "A") || !strings.Contains(err.Error(), "B") {
		t.Errorf("message should list serials: %v", err)
	}
}
