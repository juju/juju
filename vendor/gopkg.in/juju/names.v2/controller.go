// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"regexp"
)

// ControllerTagKind indicates that a tag belongs to a controller.
const ControllerTagKind = "controller"

// ControllerTag represents a tag used to describe a controller.
type ControllerTag struct {
	uuid string
}

// Lowercase letters, digits and (non-leading) hyphens.
var validControllerName = regexp.MustCompile(`^[a-z0-9]+[a-z0-9-]*$`)

// NewControllerTag returns the tag of an controller with the given controller UUID.
func NewControllerTag(uuid string) ControllerTag {
	return ControllerTag{uuid: uuid}
}

// ParseControllerTag parses an environ tag string.
func ParseControllerTag(controllerTag string) (ControllerTag, error) {
	tag, err := ParseTag(controllerTag)
	if err != nil {
		return ControllerTag{}, err
	}
	et, ok := tag.(ControllerTag)
	if !ok {
		return ControllerTag{}, invalidTagError(controllerTag, ControllerTagKind)
	}
	return et, nil
}

// String implements Tag.
func (t ControllerTag) String() string { return t.Kind() + "-" + t.Id() }

// Kind implements Tag.
func (t ControllerTag) Kind() string { return ControllerTagKind }

// Id implements Tag.
func (t ControllerTag) Id() string { return t.uuid }

// IsValidController returns whether id is a valid controller UUID.
func IsValidController(id string) bool {
	return validUUID.MatchString(id)
}

// IsValidControllerName returns whether name is a valid string safe for a controller name.
func IsValidControllerName(name string) bool {
	return validControllerName.MatchString(name)
}
