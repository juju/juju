// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
)

// StatusArgs is an argument struct used to set the agent, service, or
// workload status.
type StatusArgs struct {
	Value   string
	Message string
	Data    map[string]interface{}
	Updated time.Time
}

func newStatus(args StatusArgs) *status {
	return &status{
		Version:  1,
		Value_:   args.Value,
		Message_: args.Message,
		Data_:    args.Data,
		Updated_: args.Updated.UTC(),
	}
}

type status struct {
	Version int `yaml:"version"`

	Value_   string                 `yaml:"value"`
	Message_ string                 `yaml:"message,omitempty"`
	Data_    map[string]interface{} `yaml:"data,omitempty"`
	Updated_ time.Time              `yaml:"updated"`
}

// Value implements Status.
func (a *status) Value() string {
	return a.Value_
}

// Message implements Status.
func (a *status) Message() string {
	return a.Message_
}

// Data implements Status.
func (a *status) Data() map[string]interface{} {
	return a.Data_
}

// Updated implements Status.
func (a *status) Updated() time.Time {
	return a.Updated_
}

func importStatus(source map[string]interface{}) (*status, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Annotate(err, "status version schema check failed")
	}

	importFunc, ok := statusDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type statusDeserializationFunc func(map[string]interface{}) (*status, error)

var statusDeserializationFuncs = map[int]statusDeserializationFunc{
	1: importStatusV1,
}

func importStatusV1(source map[string]interface{}) (*status, error) {
	fields := schema.Fields{
		"value":   schema.String(),
		"message": schema.String(),
		"data":    schema.StringMap(schema.Any()),
		"updated": schema.Time(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"message": "",
		"data":    schema.Omit,
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "status v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	var data map[string]interface{}
	if sourceData, set := valid["data"]; set {
		data = sourceData.(map[string]interface{})
	}
	return &status{
		Version:  1,
		Value_:   valid["value"].(string),
		Message_: valid["message"].(string),
		Data_:    data,
		Updated_: valid["updated"].(time.Time),
	}, nil
}
