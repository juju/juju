// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
)

const (
	separator = "#"
)

// EncodedUUID represents a string-encoded type for categorizing UUIDs
// with predefined values like "app" or "unit".
type EncodedUUID string

const (

	// ApplicationUUID defines a constant EncodedUUID value used to categorize
	// application UUIDs.
	ApplicationUUID EncodedUUID = "app"

	// UnitUUID is a constant of type EncodedUUID used to categorize unit UUIDs.
	UnitUUID EncodedUUID = "unit"
)

// EncodeApplicationUUID encodes an application UUID with a marker which specify
// the fact that it is an application uuid. It helps to ship multiple uuid kind
// through watchers and interprets them.
func EncodeApplicationUUID(uuid string) string {
	return fmt.Sprintf("%s%s%s", ApplicationUUID, separator, uuid)
}

// EncodeUnitUUID encodes a unit UUID with a marker which specify
// the fact that it is a unit uuid. It helps to ship multiple uuid kind
// through watchers and interprets them.
func EncodeUnitUUID(uuid string) string {
	return fmt.Sprintf("%s%s%s", UnitUUID, separator, uuid)
}

// DecodeWatchRelationUnitChangeUUID parses an event string into its respective EncodedUUID
// and associated value, returning an error for invalid input.
func DecodeWatchRelationUnitChangeUUID(event string) (EncodedUUID, string, error) {
	values := strings.Split(event, separator)
	allowedTypes := set.NewStrings(
		string(ApplicationUUID),
		string(UnitUUID),
	)
	if len(values) != 2 || !allowedTypes.Contains(values[0]) {
		return "", "", fmt.Errorf("invalid event with uuid: %s", event)
	}
	return EncodedUUID(values[0]), values[1], nil
}
