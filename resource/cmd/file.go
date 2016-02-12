// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
)

// resourceFile associates a resource name to a filename.
type resourceFile struct {
	service  string
	name     string
	filename string
}

// parseResourceFileArg converts the provided string into a name and
// filename. The string must be in the "<name>=<filename>" format.
func parseResourceFileArg(raw string) (name string, filename string, _ error) {
	vals := strings.SplitN(raw, "=", 2)
	if len(vals) < 2 {
		msg := fmt.Sprintf("expected name=path format")
		return "", "", errors.NewNotValid(nil, msg)
	}

	name, filename = vals[0], vals[1]
	if name == "" {
		return "", "", errors.NewNotValid(nil, "missing resource name")
	}
	if filename == "" {
		return "", "", errors.NewNotValid(nil, "missing filename")
	}
	return name, filename, nil
}
