// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"regexp"
)

const ModelTagKind = "model"

// ModelTag represents a tag used to describe a model.
type ModelTag struct {
	uuid string
}

var validUUID = regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)

// Lowercase letters, digits and (non-leading) hyphens, as per LP:1568944 #5.
var validModelName = regexp.MustCompile(`^[a-z0-9]+[a-z0-9-]*$`)

// NewModelTag returns the tag of an model with the given model UUID.
func NewModelTag(uuid string) ModelTag {
	return ModelTag{uuid: uuid}
}

// ParseModelTag parses an environ tag string.
func ParseModelTag(modelTag string) (ModelTag, error) {
	tag, err := ParseTag(modelTag)
	if err != nil {
		return ModelTag{}, err
	}
	et, ok := tag.(ModelTag)
	if !ok {
		return ModelTag{}, invalidTagError(modelTag, ModelTagKind)
	}
	return et, nil
}

func (t ModelTag) String() string { return t.Kind() + "-" + t.Id() }
func (t ModelTag) Kind() string   { return ModelTagKind }
func (t ModelTag) Id() string     { return t.uuid }

// IsValidModel returns whether id is a valid model UUID.
func IsValidModel(id string) bool {
	return validUUID.MatchString(id)
}

// IsValidModelName returns whether name is a valid string safe for a model name.
func IsValidModelName(name string) bool {
	return validModelName.MatchString(name)
}
