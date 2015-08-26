// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import "time"

// FormatTime returns a string with the local time formatted
// in an arbitrary format used for status or and localized tz
// or in UTC timezone and format RFC3339 if u is specified.
func FormatTime(t *time.Time, formatISO bool) string {
	if formatISO {
		// If requested, use ISO time format.
		// The format we use is RFC3339 without the "T". From the spec:
		// NOTE: ISO 8601 defines date and time separated by "T".
		// Applications using this syntax may choose, for the sake of
		// readability, to specify a full-date and full-time separated by
		// (say) a space character.
		return t.UTC().Format("2006-01-02 15:04:05Z")
	}
	// Otherwise use local time.
	return t.Local().Format("02 Jan 2006 15:04:05Z07:00")
}
