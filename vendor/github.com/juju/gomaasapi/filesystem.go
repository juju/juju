// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import "github.com/juju/schema"

type filesystem struct {
	fstype     string
	mountPoint string
	label      string
	uuid       string
	// no idea what the mount_options are as a value type, so ignoring for now.
}

// Type implements FileSystem.
func (f *filesystem) Type() string {
	return f.fstype
}

// MountPoint implements FileSystem.
func (f *filesystem) MountPoint() string {
	return f.mountPoint
}

// Label implements FileSystem.
func (f *filesystem) Label() string {
	return f.label
}

// UUID implements FileSystem.
func (f *filesystem) UUID() string {
	return f.uuid
}

// There is no need for controller based parsing of filesystems until we need it.
// Currently the filesystem reading is only called by the Partition parsing.

func filesystem2_0(source map[string]interface{}) (*filesystem, error) {
	fields := schema.Fields{
		"fstype":      schema.String(),
		"mount_point": schema.OneOf(schema.Nil(""), schema.String()),
		"label":       schema.OneOf(schema.Nil(""), schema.String()),
		"uuid":        schema.String(),
		// TODO: mount_options when we know the type (note it can be
		// nil).
	}
	defaults := schema.Defaults{
		"mount_point": "",
		"label":       "",
	}
	checker := schema.FieldMap(fields, defaults)
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "filesystem 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	mount_point, _ := valid["mount_point"].(string)
	label, _ := valid["label"].(string)
	result := &filesystem{
		fstype:     valid["fstype"].(string),
		mountPoint: mount_point,
		label:      label,
		uuid:       valid["uuid"].(string),
	}
	return result, nil
}
