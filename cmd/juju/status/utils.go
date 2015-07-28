// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"reflect"
	"sort"
	"time"
)

// formatStatusTime returns a string with the local time
// formatted in an arbitrary format used for status or
// and localized tz or in utc timezone and format RFC3339
// if u is specified.
func formatStatusTime(t *time.Time, formatISO bool) string {
	if formatISO {
		// If requested, use ISO time format.
		// The format we use is RFC3339 without the "T". From the spec:
		// NOTE: ISO 8601 defines date and time separated by "T".
		// Applications using this syntax may choose, for the sake of
		// readability, to specify a full-date and full-time separated by
		// (say) a space character.
		return t.UTC().Format("2006-01-02 15:04:05Z")
	} else {
		// Otherwise use local time.
		return t.Local().Format("02 Jan 2006 15:04:05Z07:00")
	}
}

// sortStringsNaturally is syntactic sugar so we can do sorts in one line.
func sortStringsNaturally(s []string) []string {
	sort.Sort(naturally(s))
	return s
}

// stringKeysFromMap takes a map with keys which are strings and returns
// only the keys.
func stringKeysFromMap(m interface{}) (keys []string) {
	for _, k := range reflect.ValueOf(m).MapKeys() {
		keys = append(keys, k.String())
	}
	return
}

// recurseUnits calls the given recurseMap function on the given unit
// and its subordinates (recursively defined on the given unit).
func recurseUnits(u unitStatus, il int, recurseMap func(string, unitStatus, int)) {
	if len(u.Subordinates) == 0 {
		return
	}
	for _, uName := range sortStringsNaturally(stringKeysFromMap(u.Subordinates)) {
		unit := u.Subordinates[uName]
		recurseMap(uName, unit, il)
		recurseUnits(unit, il+1, recurseMap)
	}
}

// indent prepends a format string with the given number of spaces.
func indent(prepend string, level int, append string) string {
	return fmt.Sprintf("%s%*s%s", prepend, level, "", append)
}
