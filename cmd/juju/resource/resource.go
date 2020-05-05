// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"
	"strings"

	"github.com/juju/charm/v7/resource"
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
		msg := fmt.Sprintf("expected name=path format")
		return "", "", errors.NewNotValid(nil, msg)
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
