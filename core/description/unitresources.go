// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// UnitResource represents the revision of a resource used by a unit.
type UnitResource interface {
	Name() string
	Revision() int
}

// UnitResourceArgs is an argument struct used to specify the revision
// of a resource used by a unit.
type UnitResourceArgs struct {
	Name     string
	Revision int
}

func newUnitResource(args UnitResourceArgs) *unitResource {
	return &unitResource{
		Name_:     args.Name,
		Revision_: args.Revision,
	}
}

type unitResources struct {
	Version    int             `yaml:"version"`
	Resources_ []*unitResource `yaml:"resources"`
}

type unitResource struct {
	Name_     string `yaml:"name"`
	Revision_ int    `yaml:"revision"`
}

// Name implements UnitResource.
func (ur *unitResource) Name() string {
	return ur.Name_
}

// Revision implements UnitResource.
func (ur *unitResource) Revision() int {
	return ur.Revision_
}

func importUnitResources(source map[string]interface{}) ([]*unitResource, error) {
	checker := versionedChecker("resources")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "resources version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	if version != 1 {
		return nil, errors.NotValidf("version %d", version)
	}

	sourceList := valid["resources"].([]interface{})
	return importUnitResourceList(sourceList)
}

func importUnitResourceList(sourceList []interface{}) ([]*unitResource, error) {
	result := make([]*unitResource, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for resource %d, %T", i, value)
		}
		r, err := importUnitResourceV1(source)
		if err != nil {
			return nil, errors.Annotatef(err, "unit resource %d", i)
		}
		result = append(result, r)
	}
	return result, nil
}

func importUnitResourceV1(source map[string]interface{}) (*unitResource, error) {
	fields := schema.Fields{
		"name":     schema.String(),
		"revision": schema.Int(),
	}
	checker := schema.FieldMap(fields, nil)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "unit resource v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	return &unitResource{
		Name_:     valid["name"].(string),
		Revision_: int(valid["revision"].(int64)),
	}, nil
}
