// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type spaces struct {
	Version int      `yaml:"version"`
	Spaces_ []*space `yaml:"spaces"`
}

type space struct {
	Name_       string `yaml:"name"`
	Public_     bool   `yaml:"public"`
	ProviderID_ string `yaml:"provider-id,omitempty"`
}

// SpaceArgs is an argument struct used to create a new internal space
// type that supports the Space interface.
type SpaceArgs struct {
	Name       string
	Public     bool
	ProviderID string
}

func newSpace(args SpaceArgs) *space {
	return &space{
		Name_:       args.Name,
		Public_:     args.Public,
		ProviderID_: args.ProviderID,
	}
}

// Name implements Space.
func (s *space) Name() string {
	return s.Name_
}

// Public implements Space.
func (s *space) Public() bool {
	return s.Public_
}

// ProviderID implements Space.
func (s *space) ProviderID() string {
	return s.ProviderID_
}

func importSpaces(source map[string]interface{}) ([]*space, error) {
	checker := versionedChecker("spaces")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "spaces version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := spaceDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["spaces"].([]interface{})
	return importSpaceList(sourceList, importFunc)
}

func importSpaceList(sourceList []interface{}, importFunc spaceDeserializationFunc) ([]*space, error) {
	result := make([]*space, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for space %d, %T", i, value)
		}
		space, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "space %d", i)
		}
		result = append(result, space)
	}
	return result, nil
}

type spaceDeserializationFunc func(map[string]interface{}) (*space, error)

var spaceDeserializationFuncs = map[int]spaceDeserializationFunc{
	1: importSpaceV1,
}

func importSpaceV1(source map[string]interface{}) (*space, error) {
	fields := schema.Fields{
		"name":        schema.String(),
		"public":      schema.Bool(),
		"provider-id": schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"provider-id": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "space v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	return &space{
		Name_:       valid["name"].(string),
		Public_:     valid["public"].(bool),
		ProviderID_: valid["provider-id"].(string),
	}, nil
}
