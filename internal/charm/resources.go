// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/internal/charm/resource"
)

var resourceSchema = schema.FieldMap(
	schema.Fields{
		"type":        schema.String(),
		"filename":    schema.String(), // TODO(ericsnow) Change to "path"?
		"description": schema.String(),
	},
	schema.Defaults{
		"type":        resource.TypeFile.String(),
		"filename":    "",
		"description": "",
	},
)

func parseMetaResources(data interface{}) (map[string]resource.Meta, error) {
	if data == nil {
		return nil, nil
	}

	result := make(map[string]resource.Meta)
	for name, val := range data.(map[string]interface{}) {
		meta, err := parseResourceMeta(name, val)
		if err != nil {
			return nil, err
		}
		result[name] = meta
	}

	return result, nil
}

func validateMetaResources(resources map[string]resource.Meta) error {
	for name, res := range resources {
		if res.Name != name {
			return fmt.Errorf("mismatch on resource name (%q != %q)", res.Name, name)
		}
		if err := res.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// parseResourceMeta parses the provided data into a Meta, assuming
// that the data has first been checked with resourceSchema.
func parseResourceMeta(name string, data interface{}) (resource.Meta, error) {
	meta := resource.Meta{
		Name: name,
	}

	if data == nil {
		return meta, nil
	}
	rMap := data.(map[string]interface{})

	if val := rMap["type"]; val != nil {
		var err error
		meta.Type, err = resource.ParseType(val.(string))
		if err != nil {
			return meta, errors.Trace(err)
		}
	}

	if val := rMap["filename"]; val != nil {
		meta.Path = val.(string)
	}

	if val := rMap["description"]; val != nil {
		meta.Description = val.(string)
	}

	return meta, nil
}
