// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

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

func getVersion(source map[string]interface{}) (int, error) {
	checker := versionedChecker("")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return 0, errors.Trace(err)
	}
	valid := coerced.(map[string]interface{})
	return int(valid["version"].(int64)), nil
}
