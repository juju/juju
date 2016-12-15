// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

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

// convertToStringMap is expected to be used on a field with the schema
// checker `schema.StringMap(schema.String())`. The schema will return a
// string map as map[string]interface{}. It will make sure that the interface
// values are strings, but doesn't return them as strings. So we need to do
// that here.
func convertToStringMap(field interface{}) map[string]string {
	if field == nil {
		return nil
	}
	fieldMap := field.(map[string]interface{})
	result := make(map[string]string)
	for key, value := range fieldMap {
		result[key] = value.(string)
	}
	return result
}

// convertToMapOfMaps is expected to be used on a field with the schema
// checker `schema.StringMap(schema.StringMap(schema.Any())`.
func convertToMapOfMaps(field interface{}) map[string]map[string]interface{} {
	if field == nil {
		return nil
	}
	fieldMap := field.(map[string]interface{})
	result := make(map[string]map[string]interface{})
	for key, value := range fieldMap {
		result[key] = value.(map[string]interface{})
	}
	return result
}

// fieldToTimePtr looks for a field with the given name and converts
// it to a Time.Time, returning a pointer to it. If the field doesn't
// exist, nil is returned. This is useful for handling optional time
// fields.
func fieldToTimePtr(fields map[string]interface{}, name string) *time.Time {
	if raw, exists := fields[name]; exists {
		t := raw.(time.Time).UTC()
		return &t
	}
	return nil
}

// timePtr takes a time.Time and returns a pointer to a time.Time of
// the same (UTC) value. Nil is returned if the time is the time zero
// value. This is useful for handling optional time values.
func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	t = t.UTC()
	return &t
}
