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

// parseResourceFile converts the provided string into a resourceFile.
// The string must be in the "<name>=<filename>" format.
func parseResourceFile(service, raw string) (resourceFile, error) {
	rf := resourceFile{
		service: service,
	}

	vals := strings.SplitN(raw, "=", 2)
	if len(vals) < 2 {
		msg := fmt.Sprintf("resource given: %q, but expected name=path format", raw)
		return rf, errors.NewNotValid(nil, msg)
	}

	rf.name, rf.filename = vals[0], vals[1]
	if err := rf.validate(); err != nil {
		return rf, errors.Annotatef(err, "invalid arg %q", raw)
	}
	return rf, nil
}

// validate ensures that the resourceFile is correct.
func (rf resourceFile) validate() error {
	if rf.service == "" {
		return errors.NewNotValid(nil, "missing service name")
	}

	if rf.name == "" {
		return errors.NewNotValid(nil, "missing resource name")
	}

	if rf.filename == "" {
		return errors.NewNotValid(nil, "missing filename")
	}

	return nil
}
