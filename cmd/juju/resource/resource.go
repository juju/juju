// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"strings"

	"github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
)

// resourceValue associates a resource name to a value.
type resourceValue struct {
	application  string
	name         string
	resourceType resource.Type
	value        string
}

// parseResourceValueArg converts the provided string into a name and
// resource value. The string must be in the "<name>=<value>" format.
func parseResourceValueArg(raw string) (name string, value string, _ error) {
	vals := strings.SplitN(raw, "=", 2)
	if len(vals) < 2 {
		return "", "", errors.NewNotValid(nil, "expected name=path format")
	}

	name, value = vals[0], vals[1]
	if name == "" {
		return "", "", errors.NewNotValid(nil, "missing resource name")
	}
	if value == "" {
		return "", "", errors.NewNotValid(nil, "missing resource value")
	}
	return name, value, nil
}
