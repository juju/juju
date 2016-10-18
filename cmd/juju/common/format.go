// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"time"

	"github.com/juju/errors"
)

// LastConnection turns the *time.Time returned from the API server
// into a user facing string with either exact time or a user friendly
// string based on the args.
func LastConnection(connectionTime *time.Time, now time.Time, exact bool) string {
	if connectionTime == nil {
		return "never connected"
	}
	if exact {
		return connectionTime.String()
	}
	return UserFriendlyDuration(*connectionTime, now)
}

// UserFriendlyDuration translates a time in the past into a user
// friendly string representation relative to the "now" time argument.
func UserFriendlyDuration(when, now time.Time) string {
	since := now.Sub(when)
	// if over 24 hours ago, just say the date.
	if since.Hours() >= 24 {
		return when.Format("2006-01-02")
	}
	if since.Hours() >= 1 {
		unit := "hours"
		if int(since.Hours()) == 1 {
			unit = "hour"
		}
		return fmt.Sprintf("%d %s ago", int(since.Hours()), unit)
	}
	if since.Minutes() >= 1 {
		unit := "minutes"
		if int(since.Minutes()) == 1 {
			unit = "minute"
		}
		return fmt.Sprintf("%d %s ago", int(since.Minutes()), unit)
	}
	if since.Seconds() >= 2 {
		return fmt.Sprintf("%d seconds ago", int(since.Seconds()))
	}
	return "just now"
}

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

// ConformYAML ensures all keys of any nested maps are strings.  This is
// necessary because YAML unmarshals map[interface{}]interface{} in nested
// maps, which cannot be serialized by bson. Also, handle []interface{}.
// cf. gopkg.in/juju/charm.v4/actions.go cleanse
func ConformYAML(input interface{}) (interface{}, error) {
	switch typedInput := input.(type) {

	case map[string]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			newValue, err := ConformYAML(value)
			if err != nil {
				return nil, err
			}
			newMap[key] = newValue
		}
		return newMap, nil

	case map[interface{}]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			typedKey, ok := key.(string)
			if !ok {
				return nil, errors.New("map keyed with non-string value")
			}
			newMap[typedKey] = value
		}
		return ConformYAML(newMap)

	case []interface{}:
		newSlice := make([]interface{}, len(typedInput))
		for i, sliceValue := range typedInput {
			newSliceValue, err := ConformYAML(sliceValue)
			if err != nil {
				return nil, errors.New("map keyed with non-string value")
			}
			newSlice[i] = newSliceValue
		}
		return newSlice, nil

	default:
		return input, nil
	}
}
