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
		Version: 1,
		statusPoint: statusPoint{
			Value_:   args.Value,
			Message_: args.Message,
			Data_:    args.Data,
			Updated_: args.Updated.UTC(),
		},
	}
}

func newStatusHistory() statusHistory {
	return statusHistory{
		Version: 1,
	}
}

// statusPoint implements Status, and represents the status
// of an entity at a point in time. Used in the serialization of
// both status and statusHistory.
type statusPoint struct {
	Value_   string                 `yaml:"value"`
	Message_ string                 `yaml:"message,omitempty"`
	Data_    map[string]interface{} `yaml:"data,omitempty"`
	Updated_ time.Time              `yaml:"updated"`
}

type status struct {
	Version     int `yaml:"version"`
	statusPoint `yaml:"status"`
}

type statusHistory struct {
	Version int            `yaml:"version"`
	History []*statusPoint `yaml:"history"`
}

// Value implements Status.
func (a *statusPoint) Value() string {
	return a.Value_
}

// Message implements Status.
func (a *statusPoint) Message() string {
	return a.Message_
}

// Data implements Status.
func (a *statusPoint) Data() map[string]interface{} {
	return a.Data_
}

// Updated implements Status.
func (a *statusPoint) Updated() time.Time {
	return a.Updated_
}

func importStatus(source map[string]interface{}) (*status, error) {
	checker := versionedEmbeddedChecker("status")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotate(err, "status version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := statusDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	source = valid["status"].(map[string]interface{})
	point, err := importFunc(source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &status{
		Version:     1,
		statusPoint: point,
	}, nil
}

func importStatusHistory(history *statusHistory, source map[string]interface{}) error {
	checker := versionedChecker("history")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return errors.Annotate(err, "status version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := statusDeserializationFuncs[version]
	if !ok {
		return errors.NotValidf("version %d", version)
	}

	sourceList := valid["history"].([]interface{})
	points, err := importStatusList(sourceList, importFunc)
	if err != nil {
		return errors.Trace(err)
	}
	history.History = points
	return nil
}

func importStatusList(sourceList []interface{}, importFunc statusDeserializationFunc) ([]*statusPoint, error) {
	result := make([]*statusPoint, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for status %d, %T", i, value)
		}
		point, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "status history %d", i)
		}
		result = append(result, &point)
	}
	return result, nil
}

type statusDeserializationFunc func(map[string]interface{}) (statusPoint, error)

var statusDeserializationFuncs = map[int]statusDeserializationFunc{
	1: importStatusV1,
}

func importStatusV1(source map[string]interface{}) (statusPoint, error) {
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
		return statusPoint{}, errors.Annotatef(err, "status v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	var data map[string]interface{}
	if sourceData, set := valid["data"]; set {
		data = sourceData.(map[string]interface{})
	}
	return statusPoint{
		Value_:   valid["value"].(string),
		Message_: valid["message"].(string),
		Data_:    data,
		Updated_: valid["updated"].(time.Time),
	}, nil
}

// StatusHistory implements HasStatusHistory.
func (s *statusHistory) StatusHistory() []Status {
	var result []Status
	if count := len(s.History); count > 0 {
		result = make([]Status, count)
		for i, value := range s.History {
			result[i] = value
		}
	}
	return result
}

// SetStatusHistory implements HasStatusHistory.
func (s *statusHistory) SetStatusHistory(args []StatusArgs) {
	points := make([]*statusPoint, len(args))
	for i, arg := range args {
		points[i] = &statusPoint{
			Value_:   arg.Value,
			Message_: arg.Message,
			Data_:    arg.Data,
			Updated_: arg.Updated.UTC(),
		}
	}
	s.History = points
}

func addStatusHistorySchema(fields schema.Fields) {
	fields["status-history"] = schema.StringMap(schema.Any())
}

func (s *statusHistory) importStatusHistory(valid map[string]interface{}) error {
	return importStatusHistory(s, valid["status-history"].(map[string]interface{}))
}
