// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

// NewID returns a new payload ID.
func NewID() (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Annotate(err, "could not create new payload ID")
	}
	return uuid.String(), nil
}

// TODO(ericsnow) Add a "composite" ID type?

// BuildID composes an ID from a class and an ID.
func BuildID(class, id string) string {
	if id == "" {
		// TODO(natefinch) remove this special case when we can be sure the ID
		// is never empty (and fix the tests).
		return class
	}
	return class + "/" + id
}

// ParseID extracts the workload name and details ID from the provided string.
// The format is expected to be name/pluginID. If no separator is found, the
// whole string is assumed to be the name.
func ParseID(id string) (name, pluginID string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return id, ""
}
