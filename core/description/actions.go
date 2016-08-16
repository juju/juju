// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
)

type actions struct {
	Version  int       `yaml:"version"`
	Actions_ []*action `yaml:"actions"`
}

type action struct {
	Id_         string                 `yaml:"id"`
	Receiver_   string                 `yaml:"receiver"`
	Name_       string                 `yaml:"name"`
	Parameters_ map[string]interface{} `yaml:"parameters"`
	Enqueued_   time.Time              `yaml:"enqueued"`
	// Can't use omitempty with time.Time, it just doesn't work
	// (nothing is serialised), so use a pointer in the struct.
	Started_   *time.Time             `yaml:"started,omitempty"`
	Completed_ *time.Time             `yaml:"completed,omitempty"`
	Status_    string                 `yaml:"status"`
	Message_   string                 `yaml:"message"`
	Results_   map[string]interface{} `yaml:"results"`
}

// Id implements Action.
func (i *action) Id() string {
	return i.Id_
}

// Receiver implements Action.
func (i *action) Receiver() string {
	return i.Receiver_
}

// Name implements Action.
func (i *action) Name() string {
	return i.Name_
}

// Parameters implements Action.
func (i *action) Parameters() map[string]interface{} {
	return i.Parameters_
}

// Enqueued implements Action.
func (i *action) Enqueued() time.Time {
	return i.Enqueued_
}

// Started implements Action.
func (i *action) Started() time.Time {
	var zero time.Time
	if i.Started_ == nil {
		return zero
	}
	return *i.Started_
}

// Completed implements Action.
func (i *action) Completed() time.Time {
	var zero time.Time
	if i.Completed_ == nil {
		return zero
	}
	return *i.Completed_
}

// Status implements Action.
func (i *action) Status() string {
	return i.Status_
}

// Message implements Action.
func (i *action) Message() string {
	return i.Message_
}

// Results implements Action.
func (i *action) Results() map[string]interface{} {
	return i.Results_
}

// ActionArgs is an argument struct used to create a
// new internal action type that supports the Action interface.
type ActionArgs struct {
	Id         string
	Receiver   string
	Name       string
	Parameters map[string]interface{}
	Enqueued   time.Time
	Started    time.Time
	Completed  time.Time
	Status     string
	Message    string
	Results    map[string]interface{}
}

func newAction(args ActionArgs) *action {
	action := &action{
		Receiver_:   args.Receiver,
		Name_:       args.Name,
		Parameters_: args.Parameters,
		Enqueued_:   args.Enqueued,
		Status_:     args.Status,
		Message_:    args.Message,
		Id_:         args.Id,
		Results_:    args.Results,
	}
	if !args.Started.IsZero() {
		value := args.Started
		action.Started_ = &value
	}
	if !args.Completed.IsZero() {
		value := args.Completed
		action.Completed_ = &value
	}
	return action
}

func importActions(source map[string]interface{}) ([]*action, error) {
	checker := versionedChecker("actions")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "actions version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := actionDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["actions"].([]interface{})
	return importActionList(sourceList, importFunc)
}

func importActionList(sourceList []interface{}, importFunc actionDeserializationFunc) ([]*action, error) {
	result := make([]*action, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for action %d, %T", i, value)
		}
		action, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "action %d", i)
		}
		result = append(result, action)
	}
	return result, nil
}

type actionDeserializationFunc func(map[string]interface{}) (*action, error)

var actionDeserializationFuncs = map[int]actionDeserializationFunc{
	1: importActionV1,
}

func importActionV1(source map[string]interface{}) (*action, error) {
	fields := schema.Fields{
		"receiver":   schema.String(),
		"name":       schema.String(),
		"parameters": schema.StringMap(schema.Any()),
		"enqueued":   schema.Time(),
		"started":    schema.Time(),
		"completed":  schema.Time(),
		"status":     schema.String(),
		"message":    schema.String(),
		"results":    schema.StringMap(schema.Any()),
		"id":         schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"started":   time.Time{},
		"completed": time.Time{},
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "action v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	action := &action{
		Id_:         valid["id"].(string),
		Receiver_:   valid["receiver"].(string),
		Name_:       valid["name"].(string),
		Status_:     valid["status"].(string),
		Message_:    valid["message"].(string),
		Parameters_: valid["parameters"].(map[string]interface{}),
		Enqueued_:   valid["enqueued"].(time.Time).UTC(),
		Results_:    valid["results"].(map[string]interface{}),
	}

	started := valid["started"].(time.Time)
	if !started.IsZero() {
		started = started.UTC()
		action.Started_ = &started
	}
	completed := valid["completed"].(time.Time)
	if !started.IsZero() {
		completed = completed.UTC()
		action.Completed_ = &completed
	}
	return action, nil
}
