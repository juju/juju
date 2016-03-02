// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

func versionedChecker(name string) schema.Checker {
	fields := schema.Fields{
		"version": schema.Int(),
	}
	if name != "" {
		fields[name] = schema.List(schema.StringMap(schema.Any()))
	}
	return schema.FieldMap(fields, nil) // no defaults
}

func versionedEmbeddedChecker(name string) schema.Checker {
	fields := schema.Fields{
		"version": schema.Int(),
	}
	fields[name] = schema.StringMap(schema.Any())
	return schema.FieldMap(fields, nil) // no defaults
}

func getVersion(source map[string]interface{}) (int, error) {
	checker := versionedChecker("")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return 0, errors.Trace(err)
	}
	valid := coerced.(map[string]interface{})
	return int(valid["version"].(int64)), nil
}

func convertToStringSlice(field interface{}) []string {
	if field == nil {
		return nil
	}
	fieldSlice := field.([]interface{})
	result := make([]string, len(fieldSlice))
	for i, value := range fieldSlice {
		result[i] = value.(string)
	}
	return result
}
