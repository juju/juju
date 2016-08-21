// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// StorageConstraintArgs is an argument struct used to create a new internal
// storageconstraint type that supports the StorageConstraint interface.
type StorageConstraintArgs struct {
	Pool  string
	Size  uint64
	Count uint64
}

func newStorageConstraint(args StorageConstraintArgs) *storageconstraint {
	return &storageconstraint{
		Version: 1,
		Pool_:   args.Pool,
		Size_:   args.Size,
		Count_:  args.Count,
	}
}

type storageconstraint struct {
	Version int `yaml:"version"`

	Pool_  string `yaml:"pool"`
	Size_  uint64 `yaml:"size"`
	Count_ uint64 `yaml:"count"`
}

// Pool implements StorageConstraint.
func (s *storageconstraint) Pool() string {
	return s.Pool_
}

// Size implements StorageConstraint.
func (s *storageconstraint) Size() uint64 {
	return s.Size_
}

// Count implements StorageConstraint.
func (s *storageconstraint) Count() uint64 {
	return s.Count_
}

func importStorageConstraints(sourceMap map[string]interface{}) (map[string]*storageconstraint, error) {
	result := make(map[string]*storageconstraint)
	for key, value := range sourceMap {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for storageconstraint %q, %T", key, value)
		}
		constraint, err := importStorageConstraint(source)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[key] = constraint
	}
	return result, nil
}

// importStorageConstraint constructs a new StorageConstraint from a map representing a serialised
// StorageConstraint instance.
func importStorageConstraint(source map[string]interface{}) (*storageconstraint, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Annotate(err, "storageconstraint version schema check failed")
	}

	importFunc, ok := storageconstraintDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type storageconstraintDeserializationFunc func(map[string]interface{}) (*storageconstraint, error)

var storageconstraintDeserializationFuncs = map[int]storageconstraintDeserializationFunc{
	1: importStorageConstraintV1,
}

func importStorageConstraintV1(source map[string]interface{}) (*storageconstraint, error) {
	fields := schema.Fields{
		"pool":  schema.String(),
		"size":  schema.Uint(),
		"count": schema.Uint(),
	}
	checker := schema.FieldMap(fields, nil)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "storageconstraint v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	return &storageconstraint{
		Version: 1,
		Pool_:   valid["pool"].(string),
		Size_:   valid["size"].(uint64),
		Count_:  valid["count"].(uint64),
	}, nil
}
