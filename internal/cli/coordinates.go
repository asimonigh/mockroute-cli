package cli

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/asimonigh/mockroute-cli/internal/clierr"
)

// coordinatesPattern is the rule of the app's CoordinatesParser.
var coordinatesPattern = regexp.MustCompile(`^(-?\d{1,2}(?:[.,]\d+)?)\s*[,; ]\s*(-?\d{1,3}(?:[.,]\d+)?)$`)

// ParseCoordinates reads "lat lon", "lat,lon", "lat;lon", with a dot or comma decimal separator,
// the same rule as the app's CoordinatesParser.
func ParseCoordinates(args []string) (lat, lon float64, err error) {
	input := strings.TrimSpace(strings.Join(args, " "))
	invalid := func() (float64, float64, error) {
		return 0, 0, clierr.New("invalid_coordinates", "cannot read coordinates from %q: expected \"lat lon\" or \"lat,lon\" (latitude within ±90, longitude within ±180)", input)
	}
	m := coordinatesPattern.FindStringSubmatch(input)
	if m == nil {
		return invalid()
	}
	lat, err1 := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
	lon, err2 := strconv.ParseFloat(strings.Replace(m[2], ",", ".", 1), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return invalid()
	}
	return lat, lon, nil
}

// formatPointArg prints "lat,lon" or "lat,lon,accuracy", without spaces.
func formatPointArg(lat, lon float64, accuracy float64, hasAccuracy bool) string {
	s := strconv.FormatFloat(lat, 'f', -1, 64) + "," + strconv.FormatFloat(lon, 'f', -1, 64)
	if hasAccuracy {
		s += "," + strconv.FormatFloat(accuracy, 'f', -1, 64)
	}
	return s
}
