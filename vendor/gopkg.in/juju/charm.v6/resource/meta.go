// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
)

// Meta holds the information about a resource, as stored
// in a charm's metadata.
type Meta struct {
	// Name identifies the resource.
	Name string

	// Type identifies the type of resource (e.g. "file").
	Type Type

	// TODO(ericsnow) Rename Path to Filename?

	// Path is the relative path of the file or directory where the
	// resource will be stored under the unit's data directory. The path
	// is resolved against a subdirectory assigned to the resource. For
	// example, given an application named "spam", a resource "eggs", and a
	// path "eggs.tgz", the fully resolved storage path for the resource
	// would be:
	//   /var/lib/juju/agent/spam-0/resources/eggs/eggs.tgz
	Path string

	// Description holds optional user-facing info for the resource.
	Description string
}

// Validate checks the resource metadata to ensure the data is valid.
func (meta Meta) Validate() error {
	if meta.Name == "" {
		return errors.NewNotValid(nil, "resource missing name")
	}

	var typeUnknown Type
	if meta.Type == typeUnknown {
		return errors.NewNotValid(nil, "resource missing type")
	}
	if err := meta.Type.Validate(); err != nil {
		msg := fmt.Sprintf("invalid resource type %v: %v", meta.Type, err)
		return errors.NewNotValid(nil, msg)
	}

	if meta.Type == TypeFile && meta.Path == "" {
		// TODO(ericsnow) change "filename" to "path"
		return errors.NewNotValid(nil, "resource missing filename")
	}
	if meta.Type == TypeFile {
		if strings.Contains(meta.Path, "/") {
			msg := fmt.Sprintf(`filename cannot contain "/" (got %q)`, meta.Path)
			return errors.NewNotValid(nil, msg)
		}
		// TODO(ericsnow) Constrain Path to alphanumeric?
	}

	return nil
}
