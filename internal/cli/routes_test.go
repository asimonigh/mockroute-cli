package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

const (
	idCol   = "11111111-1111-1111-1111-111111111111"
	idAlpes = "22222222-2222-2222-2222-222222222222"
	idEte1  = "33333333-3333-3333-3333-333333333333"
	idEte2  = "44444444-4444-4444-4444-444444444444"
	routesJ = `{"routes":[` +
		`{"id":"` + idCol + `","name":"Col du Tourmalet","distance_m":1000,"duration_s":60,"mode":"ROAD","recorded_times":false},` +
		`{"id":"` + idAlpes + `","name":"Alpes","distance_m":2000,"duration_s":120,"mode":"ROAD","recorded_times":true}]}`
)

func TestResolveRoute(t *testing.T) {
	routes := []savedRoute{
		{ID: idCol, Name: "Col du Tourmalet"},
		{ID: idAlpes, Name: "Alpes"},
		{ID: idEte1, Name: "Été"},
		{ID: "x", Name: idAlpes},
	}
	cases := []struct{ query, want string }{
		{idCol, idCol},
		{"col DU tourmalet", idCol},
		{"été", idEte1},
		{idAlpes, idAlpes}, // the id wins over a name equal to this id
	}
	for _, c := range cases {
		got, err := resolveRoute(routes, c.query)
		if err != nil || got != c.want {
			t.Errorf("%q: got %q, %v, want %q", c.query, got, err, c.want)
		}
	}

	dup := []savedRoute{{ID: idEte1, Name: "Été"}, {ID: idEte2, Name: "ÉTÉ"}, {ID: idCol, Name: "Col"}}
	_, err := resolveRoute(dup, "été")
	ce, ok := err.(*clierr.Error)
	if !ok || ce.Code != "ambiguous_route" || !reflect.DeepEqual(ce.Details["ids"], []string{idEte1, idEte2}) {
		t.Fatalf("ambiguous: %#v", err)
	}

	_, err = resolveRoute(routes, "nope")
	if ce, ok := err.(*clierr.Error); !ok || ce.Code != "route_not_found" || !strings.Contains(ce.Message, `"nope"`) {
		t.Fatalf("not found: %#v", err)
	}
}

// routePlayRunner scripts play --route: setup status, before-start status, then the given states.
func routePlayRunner(routesAnswer string, speedArg string, states ...string) *fakeRunner {
	f := playRunner("34", states...)
	f.responses[provider("routes")] = resp{out: bundle(routesAnswer)}
	f.responses[provider("play_saved")+" --arg "+speedArg] = resp{out: bundle(loadOK)}
	return f
}

func TestPlayRouteHappyPath(t *testing.T) {
	f := routePlayRunner(routesJ, idCol+",30", "simulating")
	code, out, _ := runEnv(t, f, fakeClock, "play", "--route", "col du tourmalet", "--speed", "30")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	want := []string{
		"devices -l",
		"-s S1 shell pm list packages " + pkg,
		provider("version"),
		provider("status"),
		provider("routes"),
		provider("play_saved") + " --arg " + idCol + ",30",
		provider("status"),
		"-s S1 shell am start -n " + pkg + "/com.simonigh.fakemygpslocation.cli.CliStartActivity",
		provider("status"),
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls:\n%s", strings.Join(f.calls, "\n"))
	}
	for _, c := range f.calls {
		if strings.Contains(strings.ToLower(c), "tourmalet") {
			t.Fatalf("the name reached adb: %s", c)
		}
	}
	m := decode(t, out)
	if m["state"] != "simulating" || m["pace_source"] != "gpx_times" {
		t.Fatalf("got %s", out)
	}
}

func TestPlayRouteWithoutSpeed(t *testing.T) {
	f := routePlayRunner(routesJ, idAlpes, "simulating")
	if code, out, _ := runEnv(t, f, fakeClock, "play", "--route", idAlpes); code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPlayRouteWithFile(t *testing.T) {
	f := playRunner("34")
	code, out, _ := run(t, f, "play", "--route", "Alpes", gpxFile(t))
	if code != 1 || errCode(t, out) != "usage" || !strings.Contains(out, "play takes a GPX file or --route, not both") || len(f.calls) != 0 {
		t.Fatalf("exit %d: %s calls %v", code, out, f.calls)
	}
}

func TestPlayRouteInvalidSpeed(t *testing.T) {
	f := playRunner("34")
	code, out, _ := run(t, f, "play", "--route", "Alpes", "--speed", "0")
	if code != 6 || errCode(t, out) != "invalid_speed" || len(f.calls) != 0 {
		t.Fatalf("exit %d: %s calls %v", code, out, f.calls)
	}
}

func TestPlayRouteNotFound(t *testing.T) {
	f := routePlayRunner(routesJ, "x", "simulating")
	code, out, _ := run(t, f, "play", "--route", "Inconnu")
	if code != 6 || errCode(t, out) != "route_not_found" || f.called("play_saved") || f.called("am start") {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPlayRouteAmbiguous(t *testing.T) {
	dup := `{"routes":[{"id":"` + idEte1 + `","name":"Été"},{"id":"` + idEte2 + `","name":"été"}]}`
	f := routePlayRunner(dup, "x", "simulating")
	code, out, _ := run(t, f, "play", "--route", "ÉTÉ")
	if code != 6 || errCode(t, out) != "ambiguous_route" || f.called("play_saved") {
		t.Fatalf("exit %d: %s", code, out)
	}
	ids, _ := decode(t, out)["error"].(map[string]any)["ids"].([]any)
	if len(ids) != 2 || ids[0] != idEte1 || ids[1] != idEte2 {
		t.Fatalf("ids: %s", out)
	}
}

func TestPlayRouteProRequired(t *testing.T) {
	f := routePlayRunner(routesJ, idCol, "simulating")
	f.responses[provider("play_saved")+" --arg "+idCol] = resp{out: bundle(`{"error":{"code":"pro_required","message":"Pro"}}`)}
	code, out, _ := run(t, f, "play", "--route", "Col du Tourmalet")
	if code != 4 || errCode(t, out) != "pro_required" || f.called("am start") {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestPlayRouteOldApp(t *testing.T) {
	old := `{"error":{"code":"unknown_method","message":"x"}}`
	f := playRunner("34")
	f.responses[provider("routes")] = resp{out: bundle(old)}
	code, out, _ := run(t, f, "play", "--route", "Alpes")
	if code != 3 || errCode(t, out) != "app_outdated" || !strings.Contains(out, "cannot play a saved route from the CLI") {
		t.Fatalf("routes: exit %d: %s", code, out)
	}
	g := routePlayRunner(routesJ, idAlpes)
	g.responses[provider("play_saved")+" --arg "+idAlpes] = resp{out: bundle(old)}
	code, out, _ = run(t, g, "play", "--route", "Alpes")
	if code != 3 || errCode(t, out) != "app_outdated" || g.called("am start") {
		t.Fatalf("play_saved: exit %d: %s", code, out)
	}
}

func TestRoutesOldApp(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("routes")] = resp{out: bundle(`{"error":{"code":"unknown_method","message":"x"}}`)}
	code, out, _ := run(t, f, "routes")
	if code != 3 || errCode(t, out) != "app_outdated" || !strings.Contains(out, "cannot list saved routes from the CLI") {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestRoutesPassThrough(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("routes")] = resp{out: bundle("{\n \"routes\": [ {\"id\":\"a\",\"name\":\"A\"} ]\n}")}
	code, out, _ := run(t, f, "routes")
	if code != 0 || out != `{"routes":[{"id":"a","name":"A"}]}`+"\n" {
		t.Fatalf("exit %d: %q", code, out)
	}
}

func TestRoutesProRequired(t *testing.T) {
	f := &fakeRunner{responses: baseResponses()}
	f.responses[provider("routes")] = resp{out: bundle(`{"error":{"code":"pro_required","message":"Pro"}}`)}
	code, out, _ := run(t, f, "routes")
	if code != 4 || errCode(t, out) != "pro_required" {
		t.Fatalf("exit %d: %s", code, out)
	}
}
