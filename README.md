# mockroute

`mockroute` drives the MockRoute Android app from a computer, over `adb`. It plays a GPX file or a saved route as a simulated GPS route on a connected phone, changes its speed, pauses and resumes it, or holds a fixed position, reports the state of MockRoute and stops it. Every command prints one line of JSON on stdout (except `--help` and a bare `mockroute`, which print usage on stderr) and exits with a code that tells what went wrong, so scripts and AI agents can drive it.

## Requirements

- `adb` on the machine (Android platform-tools), with USB debugging enabled and authorized on the phone. The CLI looks for `adb` on the `PATH`, then in `ANDROID_HOME`, `ANDROID_SDK_ROOT` and the default SDK location of the OS. `--adb` overrides it.
- MockRoute Pro, or the `internal` build of the app, installed on the phone. `routes`, `play`, `point`, `speed`, `pause` and `resume` need Pro; `setup`, `status`, `stop` and `version` work without it.
- Android 7 or later.
- The phone unlocked with the screen on while a simulation starts.

## Installation

Download the archive for your platform, extract it and put `mockroute` on your `PATH`.

| Platform | Download |
|---|---|
| macOS, Apple silicon | [mockroute_darwin_arm64.tar.gz](https://github.com/asimonigh/mockroute-cli/releases/latest/download/mockroute_darwin_arm64.tar.gz) |
| macOS, Intel | [mockroute_darwin_amd64.tar.gz](https://github.com/asimonigh/mockroute-cli/releases/latest/download/mockroute_darwin_amd64.tar.gz) |
| Linux, x86-64 | [mockroute_linux_amd64.tar.gz](https://github.com/asimonigh/mockroute-cli/releases/latest/download/mockroute_linux_amd64.tar.gz) |
| Linux, ARM64 | [mockroute_linux_arm64.tar.gz](https://github.com/asimonigh/mockroute-cli/releases/latest/download/mockroute_linux_arm64.tar.gz) |
| Windows, x86-64 | [mockroute_windows_amd64.zip](https://github.com/asimonigh/mockroute-cli/releases/latest/download/mockroute_windows_amd64.zip) |
| Windows, ARM64 | [mockroute_windows_arm64.zip](https://github.com/asimonigh/mockroute-cli/releases/latest/download/mockroute_windows_arm64.zip) |

```sh
curl -L https://github.com/asimonigh/mockroute-cli/releases/latest/download/mockroute_darwin_arm64.tar.gz | tar -xz mockroute
```

Each release also has a `checksums.txt`.

### macOS quarantine

A file downloaded with a browser is quarantined by macOS, which then refuses to run it. Remove the flag once:

```sh
xattr -d com.apple.quarantine mockroute
```

Downloading with `curl`, as above, does not set the flag.

## Usage

```
mockroute [--serial S] [--package P] [--adb PATH] <command>
```

| Command | What it does |
|---|---|
| `setup` | Grants what `adb` can grant (location, notifications, mock location app, battery) and reports every step. |
| `play <file.gpx> [--speed km/h] [--wait] [--timeout duration]` | Runs `setup`, loads the GPX file and starts the simulation. |
| `routes` | Lists the routes saved in the app. |
| `play --route <name\|id> [--speed km/h] [--wait] [--timeout duration]` | Runs `setup`, then plays a saved route. See [Saved routes](#saved-routes). |
| `point <lat> <lon> [--accuracy m]` | Runs `setup`, then holds a fixed position (state `static`). Also accepts `<lat>,<lon>`. |
| `speed <km/h>` | Changes the speed of the running or paused route. See [Speed, pause and resume](#speed-pause-and-resume). |
| `pause` | Pauses the running route. |
| `resume` | Resumes the paused route. |
| `status` | Prints the state of MockRoute. |
| `stop` | Stops the running route or fixed point, whether it was started from the app or from the CLI. |
| `version` | Prints `{"cli": "...", "app": {...}}`. `app` is absent when no device or no MockRoute app answers. |

Global options:

| Option | Meaning |
|---|---|
| `--serial S` | Device serial, required when several devices are connected. |
| `--package P` | MockRoute variant, required when several are installed: `com.simonigh.fakemygpslocation`, `.internal`, `.debug` or `.staging`. |
| `--adb PATH` | Path of the `adb` binary. |

`play` options:

| Option | Meaning |
|---|---|
| `--speed km/h` | Constant speed, from 1 to 300. Without it, the times of the GPX file are used when they are reliable, otherwise 50 km/h. The answer's `pace_source` says which one applied (`speed_arg`, `gpx_times` or `default`). |
| `--wait` | Wait until the route is finished (state `arrived`) before returning. Returns `stopped` (exit 7) when the simulation is stopped meanwhile, or when a fixed-position mock (`static`) takes over, and `protocol_error` on any other unexpected state. |
| `--timeout duration` | With `--wait`, give up after this long, for example `30m`. The simulation keeps running. No limit by default. |

`play` runs `setup` first, then replaces any simulation already running. A `play` that does not start the simulation within a few seconds (nothing staged, phone locked) reports `start_failed` with the last `status` in `last_status`; an `arrived` route left from a previous `play` is not mistaken for the new start.

### `point`

`point` runs `setup`, then makes the phone report one fixed position until `stop`. It needs MockRoute Pro, like `play`. The answer is the `status` object, with `state` `static`.

```sh
mockroute point 43.6045 1.444
mockroute point 43.6045,-1.444 --accuracy 25
mockroute point 43,6045 1,444          # a comma works as the decimal separator
```

The coordinates are `lat lon`, `lat,lon` or `lat;lon`, latitude within ±90 and longitude within ±180. A negative number needs no `--`. `--accuracy m` sets the reported accuracy in meters, above 0 and at most 10000; without it the app uses 4 m. When a fixed point is already running, `point` moves it without restarting anything. A `point` that does not reach `static` within 12 seconds (a point started after a route can take several seconds) reports `start_failed` with the last `status` in `last_status`. An app without this command answers `app_outdated`.

### Saved routes

`routes` prints the routes saved in the app:

```json
{"routes": [{"id": "6f1c...", "name": "Tour des quais", "distance_m": 12840.0, "duration_s": 925, "mode": "road", "recorded_times": false}]}
```

`mode` is `straight_line`, `road` or `imported`. `recorded_times` is true when the route has a time for every point.

`play --route <name|id>` takes an id or a name. An identical id wins, then a name matches ignoring case. When several routes share the name, it fails with `ambiguous_route` and lists their `ids`: pass one of them to `--route`. No match gives `route_not_found`. `--route` cannot be combined with a GPX file, and `--speed`, `--wait` and `--timeout` work as for a GPX file.

The pace is `--speed` when given, otherwise the recorded times when `recorded_times` is true and every point has a time, otherwise 50 km/h. The answer's `pace_source` says which one applied. The map shows the name of the route. Only the id is sent to the phone.

### Speed, pause and resume

```sh
mockroute speed 90
mockroute pause
mockroute resume
```

They answer the `status` object and do not run `setup`. `speed` takes 1 to 300 km/h and works on a running or paused route, started from the app or from the CLI. On a paused route, the new speed applies on `resume`. On a replay of recorded times, it switches to a constant speed. `pause` and `resume` succeed when the route is already in that state. When no route is playing or paused (state `idle`, `arrived`, `static` or `failed`), all three fail with `no_route` (exit 7). An app without these commands answers `app_outdated`.

### `setup`

`setup` checks each step before acting and does not redo a finished one. It prints:

```json
{"ready": true, "steps": {"location": "done", "notifications": "applied", "developer_mode": "done", "mock_app": "applied", "battery": "done"}, "errors": {}}
```

Each step is `done` (already in place), `applied` (done by the CLI) or `failed` (the `errors` map has the `adb` message). `location` and `mock_app` are required: if one fails, `setup` exits with `setup_failed`. `developer_mode` cannot be done through `adb`. On MIUI and HyperOS, turn on "USB debugging (Security settings)" in the developer options if `pm grant` is refused.

### `status`

`status` prints the object of the app, `play` prints the same object after the start (or the arrival with `--wait`) plus `pace_source`.

```json
{
  "state": "idle | simulating | paused | arrived | failed | static",
  "pro": true,
  "setup": { "ready": true, "missing": ["notifications"] },
  "mock_settings": { "targets": ["system_providers", "fused"], "fixed_altitude_m": 150.0 },
  "route": {
    "points": 812,
    "distance_m": 12840.0,
    "traveled_m": 4100.0,
    "progress": 0.32,
    "estimated_duration_s": 925,
    "pace": { "type": "constant_speed", "speed_kmh": 50.0 }
  },
  "position": { "lat": 43.6045, "lon": 1.444, "speed_mps": 13.9 },
  "started_at": "2026-11-18T10:00:00Z",
  "error": { "code": "mock_failed", "message": "...", "target": "fused" }
}
```

- `route`, `position` and `started_at` are present when the state is `simulating`, `paused` or `arrived`. `position` is also present for `static`, a static mock started from the app.
- `error` is present only for `failed`. `target` is the mock target when known.
- For the times of the GPX file, `pace` is `{ "type": "recorded_times" }`.
- `arrived` keeps the last simulated point until `stop`.
- The keys of `setup.missing` are `location`, `notifications`, `developer_mode`, `mock_app` and `battery`.

### Errors and exit codes

On failure, the CLI prints `{"error": {"code": "...", "message": "..."}}` on stdout, with extra keys for some codes: `last_status` (`start_failed`), `app_error` (`mock_failed`), `steps` and `errors` (`setup_failed`), `missing` (`setup_incomplete`), `ids` (`ambiguous_route`).

| Exit code | Meaning | Error codes |
|---|---|---|
| 0 | Success | |
| 1 | Bad command line or file | `usage`, `file_not_found` |
| 2 | adb or device problem | `adb_not_found`, `no_device`, `multiple_devices`, `device_unauthorized`, `device_locked`, `adb_failed` |
| 3 | App or protocol problem | `app_not_installed`, `multiple_packages`, `app_outdated`, `cli_outdated`, `protocol_error`, `unknown_method`; any other code, such as `app_not_ready` or an internal error |
| 4 | Pro required | `pro_required` |
| 5 | Setup not done | `setup_incomplete`, `setup_failed` |
| 6 | Invalid input | `invalid_gpx`, `invalid_speed`, `invalid_coordinates`, `invalid_accuracy`, `route_not_found`, `ambiguous_route` |
| 7 | Simulation problem | `start_failed`, `mock_failed`, `stopped`, `no_route` |
| 8 | Timeout | `timeout` |

## Using it from an AI agent

A typical session:

```sh
mockroute status                       # is the phone ready? check "pro" and "setup.ready"
mockroute play route.gpx --speed 60    # starts and returns once the simulation runs
mockroute status                       # poll: state, route.progress, position
mockroute stop
```

To drive a route saved in the app:

```sh
mockroute routes                       # ids, names, distance, recorded_times
mockroute play --route "Tour des quais"
mockroute speed 90                     # change the speed while it runs
mockroute pause                        # and resume with: mockroute resume
```

AI agents run their tools under a time limit, often a few minutes. For a long route, do not pass `--wait`: `play` returns as soon as the simulation runs, with `route.estimated_duration_s` in its answer. Read that value, then poll `status` until `state` is `arrived`. Use `--wait` only when the route is shorter than the limit of your tool, and add `--timeout` to stay under it.

`app_outdated` can appear for a few seconds right after installing or updating MockRoute, or rebooting the phone: retry.

Keep the phone unlocked with the screen on: a locked phone can make the start fail (`device_locked` or `start_failed`). Unlock it and run the command again.

## Security model

The app exposes its CLI provider to the `adb` shell only: it refuses every other caller, including other apps on the phone. The activity that starts a simulation only acts on a request that the provider prepared just before, so it does nothing when opened any other way. `routes`, `play`, `point`, `speed`, `pause` and `resume` need MockRoute Pro (or the `internal` build); without it the answer is `pro_required` and no paywall opens. `status`, `stop` and `version` work without Pro.

## License

[MIT](LICENSE)
