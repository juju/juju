// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource

import (
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	internalerrors "github.com/juju/juju/internal/errors"
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
		return internalerrors.Errorf("resource missing name").Add(coreerrors.NotValid)
	}

	var typeUnknown Type
	if meta.Type == typeUnknown {
		return internalerrors.Errorf("resource missing type").Add(coreerrors.NotValid)
	}
	if err := meta.Type.Validate(); err != nil {
		return internalerrors.Errorf("invalid resource type %v: %v", meta.Type, err).Add(coreerrors.NotValid)
	}

	if meta.Type == TypeFile && meta.Path == "" {
		// TODO(ericsnow) change "filename" to "path"
		return internalerrors.Errorf("resource missing filename").Add(coreerrors.NotValid)
	}
	if meta.Type == TypeFile {
		if strings.Contains(meta.Path, "/") {
			return internalerrors.Errorf(`filename cannot contain "/" (got %q)`, meta.Path).Add(coreerrors.NotValid)
		}
		// TODO(ericsnow) Constrain Path to alphanumeric?
	}

	return nil
}
