package adb

import (
	"strings"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

// Device is one line of `adb devices`.
type Device struct{ Serial, State string }

// ParseDevices parses the output of `adb devices` (with or without -l).
func ParseDevices(out string) []Device {
	var devices []Device
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		devices = append(devices, Device{Serial: fields[0], State: fields[1]})
	}
	return devices
}

// SelectDevice picks the device to use, by serial or the only one ready.
func SelectDevice(devices []Device, serial string) (Device, error) {
	if serial != "" {
		for _, d := range devices {
			if d.Serial != serial {
				continue
			}
			switch d.State {
			case "device":
				return d, nil
			case "unauthorized":
				return Device{}, clierr.New("device_unauthorized", "device %s is unauthorized: accept the USB debugging prompt on the phone", serial)
			default:
				return Device{}, clierr.New("no_device", "device %s is %s", serial, d.State)
			}
		}
		return Device{}, clierr.New("no_device", "device %s not found", serial)
	}
	var ready []Device
	unauthorized := false
	for _, d := range devices {
		switch d.State {
		case "device":
			ready = append(ready, d)
		case "unauthorized":
			unauthorized = true
		}
	}
	switch {
	case len(ready) == 1:
		return ready[0], nil
	case len(ready) > 1:
		serials := make([]string, len(ready))
		for i, d := range ready {
			serials[i] = d.Serial
		}
		return Device{}, clierr.New("multiple_devices", "several devices connected (%s): pass --serial", strings.Join(serials, ", "))
	case unauthorized:
		return Device{}, clierr.New("device_unauthorized", "device is unauthorized: accept the USB debugging prompt on the phone")
	}
	return Device{}, clierr.New("no_device", "no device connected")
}
